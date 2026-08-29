package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strings"
)

//go:embed templates
var templateFS embed.FS

var funcs = template.FuncMap{
	"inc":  func(i int) int { return i + 1 },
	"pips": pips,
}

// Two template sets rather than one: "body" is defined twice (home and room),
// so they cannot live in the same set.
var (
	homeTmpl  = mustParse("templates/layout.html", "templates/home.html")
	roomTmpl  = mustParse("templates/layout.html", "templates/room.html", "templates/panel.html")
	panelTmpl = mustParse("templates/room.html", "templates/panel.html")
)

func mustParse(files ...string) *template.Template {
	t, err := template.New("layout").Funcs(funcs).ParseFS(templateFS, files...)
	if err != nil {
		log.Fatalf("parse templates %v: %v", files, err)
	}
	return t
}

func render(w http.ResponseWriter, status int, t *template.Template, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

// pips returns the nine cells of a die face, true where a pip belongs.
func pips(value int) []bool {
	layouts := map[int][]int{
		1: {4}, 2: {0, 8}, 3: {0, 4, 8}, 4: {0, 2, 6, 8}, 5: {0, 2, 4, 6, 8}, 6: {0, 2, 3, 5, 6, 8},
	}
	if value < 1 || value > 6 {
		value = 1
	}
	out := make([]bool, 9)
	for _, i := range layouts[value] {
		out[i] = true
	}
	return out
}

/* ------------------------------------------------------------ view models --- */

// Everything below shapes room state for a template. Keeping the arithmetic
// here rather than in the markup means the templates stay readable and the
// logic stays testable.

type homeView struct {
	User         *D3bitUser
	Lobbies      []lobbyView
	PlayerCounts []int
	GoogleURL    string
	Error        string
	LoginMsg     string
}

type lobbyView struct {
	Code       string
	Players    int
	MaxPlayers int
}

type choiceView struct {
	Index  int
	Letter string
	Text   string
	Class  string
}

type questionView struct {
	Index      int
	Number     int
	Prompt     string
	Choices    []choiceView
	Answered   bool
	WasCorrect bool
	Seeded     bool
	Fact       string
	Source     string
	SourceURL  string
	Waiting    int
	Pct        int
}

type reviewView struct {
	Prompt string
	Answer string
	Fact   string
	Source string
}

type roomView struct {
	Room        *Room
	Me          string
	CalaEnabled bool
	Flash       string

	Picking       string
	PickerName    string
	IsTopicPicker bool
	IsMyPick      bool
	IHaveRolled   bool
	FillPct       int

	SubTopics   []subTopicView
	Suggestions []string
	Question    *questionView
	Ranked      []*Player
	Review      []reviewView
	HasURLs     bool
}

type subTopicView struct {
	SubTopic
	OwnerName string
}

// buildRoomView snapshots the room for one player. The caller must not hold
// the room lock; this takes it.
func (s *Server) buildRoomView(room *Room, me string, flash string) *roomView {
	room.mu.Lock()
	defer room.mu.Unlock()
	return s.roomViewLocked(room, me, flash)
}

func (s *Server) roomViewLocked(room *Room, me, flash string) *roomView {
	v := &roomView{
		Room:        room,
		Me:          me,
		CalaEnabled: s.cala.Enabled(),
		Flash:       flash,
		FillPct:     len(room.Players) * 100 / max(1, room.MaxPlayer),
	}

	switch room.Phase {
	case PhaseTopic:
		v.Picking = room.TopicPicker()
	case PhaseSubTopics:
		v.Picking = room.NextSubTopicPicker()
	}
	v.IsTopicPicker = room.Phase == PhaseTopic && room.TopicPicker() == me
	v.IsMyPick = room.Phase == PhaseSubTopics && room.NextSubTopicPicker() == me
	if p := room.find(v.Picking); p != nil {
		v.PickerName = p.DisplayName
	} else {
		v.PickerName = "the next player"
	}
	if p := room.find(me); p != nil {
		v.IHaveRolled = p.Roll != 0
	}

	for _, st := range room.SubTopics {
		sv := subTopicView{SubTopic: st}
		if owner := room.find(st.ClaimedBy); owner != nil {
			sv.OwnerName = owner.DisplayName
		}
		v.SubTopics = append(v.SubTopics, sv)
	}

	if room.Phase == PhaseQuiz && room.Current < len(room.Questions) {
		v.Question = questionViewFor(room, room.Questions[room.Current], me)
	}

	if room.Phase == PhaseResults {
		v.Ranked = append([]*Player{}, room.Players...)
		sort.SliceStable(v.Ranked, func(i, j int) bool { return v.Ranked[i].Score > v.Ranked[j].Score })
		for _, q := range room.Questions {
			answer := ""
			if q.Answer >= 0 && q.Answer < len(q.Options) {
				answer = q.Options[q.Answer]
			}
			src := q.Source
			if q.SourceURL != "" {
				src = q.SourceURL
				v.HasURLs = true
			}
			v.Review = append(v.Review, reviewView{Prompt: q.Prompt, Answer: answer, Fact: q.Fact, Source: src})
		}
	}
	return v
}

func questionViewFor(room *Room, q *Question, me string) *questionView {
	qv := &questionView{
		Index:     q.Index,
		Number:    q.Index + 1,
		Prompt:    q.Prompt,
		Seeded:    q.SeededBy == me,
		Fact:      q.Fact,
		Source:    q.Source,
		SourceURL: q.SourceURL,
		Waiting:   len(room.Players) - len(q.Answers),
		Pct:       q.Index * 100 / max(1, len(room.Questions)),
	}
	if p := room.find(me); p != nil {
		qv.Answered = p.answered[q.Index]
	}

	myChoice := -1
	for _, a := range q.Answers {
		if a.PlayerID == me {
			myChoice = a.Choice
			qv.WasCorrect = a.Correct
		}
	}

	for i, opt := range q.Options {
		c := choiceView{Index: i, Letter: string(rune('A' + i)), Text: opt}
		// The right answer is only ever sent to a player who has committed.
		if qv.Answered {
			switch {
			case i == q.Answer:
				c.Class = "correct"
			case i == myChoice:
				c.Class = "wrong"
			}
		}
		qv.Choices = append(qv.Choices, c)
	}
	return qv
}

func titleCode(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }
