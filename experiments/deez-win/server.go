package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// sessionSource resolves a request to a player identity: the real D3BIT proxy,
// or the local dev stand-in.
type sessionSource interface {
	Routes(mux *http.ServeMux)
	User(r *http.Request) *D3bitUser
}

type Server struct {
	auth     sessionSource
	d3bitURL string
	origin   string
	store    *Store
	cala     *Cala
	fal      *FAL
	board    *Leaderboard
}

// sendMagicLink delegates to whichever auth backend is wired up.
func (s *Server) sendMagicLink(ctx context.Context, email string) error {
	sender, ok := s.auth.(interface {
		SendMagicLink(context.Context, string, string) error
	})
	if !ok {
		return errors.New("Sign-in is not available.")
	}
	if email == "" {
		return errors.New("Enter an email first.")
	}
	return sender.SendMagicLink(ctx, email, s.origin+"/auth/callback")
}

func (s *Server) Routes(mux *http.ServeMux) {
	s.auth.Routes(mux)

	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /lobbies", s.handleLobbies)
	mux.HandleFunc("GET /leaderboard", s.handleLeaderboard)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("POST /logout", s.handleLogout)
	mux.HandleFunc("POST /rooms", s.handleCreateRoom)
	mux.HandleFunc("POST /join", s.handleJoin)
	mux.HandleFunc("GET /rooms/{code}", s.handleRoomPage)
	mux.HandleFunc("GET /rooms/{code}/panel", s.handlePanel)
	mux.HandleFunc("POST /rooms/{code}/roll", s.handleRoll)
	mux.HandleFunc("POST /rooms/{code}/topic", s.handleTopic)
	mux.HandleFunc("POST /rooms/{code}/subtopic", s.handleSubTopic)
	mux.HandleFunc("POST /rooms/{code}/answer", s.handleAnswer)
}

// session guarantees a player identity, creating an anonymous one on the fly.
// Nobody should ever meet a login wall just to look at the home page.
func (s *Server) session(w http.ResponseWriter, r *http.Request) *D3bitUser {
	if u := s.auth.User(r); u != nil {
		return u
	}
	if bootstrap, ok := s.auth.(interface {
		Bootstrap(http.ResponseWriter, *http.Request) *D3bitUser
	}); ok {
		return bootstrap.Bootstrap(w, r)
	}
	return nil
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	u := s.session(w, r)
	render(w, http.StatusOK, homeTmpl, "layout", s.homeView(u, ""))
}

func (s *Server) homeView(u *D3bitUser, msg string) homeView {
	rooms := s.store.PublicRooms()
	sort.Slice(rooms, func(i, j int) bool { return rooms[i].CreatedAt.After(rooms[j].CreatedAt) })

	lobbies := make([]lobbyView, 0, len(rooms))
	for _, room := range rooms {
		room.mu.Lock()
		lobbies = append(lobbies, lobbyView{Code: room.Code, Players: len(room.Players), MaxPlayers: room.MaxPlayer})
		room.mu.Unlock()
	}

	return homeView{
		User:         u,
		Top:          s.board.Top(10),
		MyRank:       rankOf(s.board, u),
		Lobbies:      lobbies,
		PlayerCounts: []int{2, 3, 4},
		GoogleURL:    s.d3bitURL + "/auth/google?redirect=" + url.QueryEscape(s.origin+"/auth/callback"),
		LoginMsg:     msg,
	}
}

func (s *Server) handleLobbies(w http.ResponseWriter, r *http.Request) {
	render(w, http.StatusOK, homeTmpl, "lobbies", s.homeView(s.auth.User(r), ""))
}

func (s *Server) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	render(w, http.StatusOK, homeTmpl, "leaderboard", s.homeView(s.auth.User(r), ""))
}

func rankOf(b *Leaderboard, u *D3bitUser) int {
	if u == nil {
		return 0
	}
	return b.Rank(u.ID)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	email := strings.TrimSpace(r.FormValue("email"))
	msg := "Check your inbox for the sign-in link."
	if err := s.sendMagicLink(r.Context(), email); err != nil {
		msg = err.Error()
	}
	render(w, http.StatusOK, homeTmpl, "account", s.homeView(s.auth.User(r), msg))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if lo, ok := s.auth.(interface {
		Logout(http.ResponseWriter, *http.Request)
	}); ok {
		lo.Logout(w, r)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	u := s.session(w, r)
	if u == nil {
		s.fail(w, r, "Could not start a session. Is D3BIT reachable?")
		return
	}
	r.ParseForm()

	room := s.store.Create(r.FormValue("public") == "1", atoi(r.FormValue("max_players"), 3), 8)
	if _, err := room.Join(u); err != nil {
		s.fail(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/rooms/"+room.Code, http.StatusSeeOther)
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	u := s.session(w, r)
	if u == nil {
		s.fail(w, r, "Could not start a session. Is D3BIT reachable?")
		return
	}
	r.ParseForm()

	room, err := s.store.Get(titleCode(r.FormValue("code")))
	if err != nil {
		s.fail(w, r, "No room with that code.")
		return
	}
	if _, err := room.Join(u); err != nil {
		msg := "Could not join that room."
		switch err {
		case ErrRoomFull:
			msg = "That room is full."
		case ErrBadPhase:
			msg = "That game has already started."
		}
		s.fail(w, r, msg)
		return
	}
	http.Redirect(w, r, "/rooms/"+room.Code, http.StatusSeeOther)
}

// fail re-renders the home page with a message. htmx 4 swaps error responses by
// default, so returning real HTML here means the user sees the problem in place.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, msg string) {
	v := s.homeView(s.auth.User(r), "")
	v.Error = msg
	render(w, http.StatusUnprocessableEntity, homeTmpl, "layout", v)
}

func (s *Server) renderRoom(w http.ResponseWriter, r *http.Request, room *Room, me, flash string) {
	v := s.buildRoomView(room, me, flash)
	v.Suggestions = s.suggestions(r.Context(), room)
	render(w, http.StatusOK, roomTmpl, "layout", v)
}

func (s *Server) renderPanel(w http.ResponseWriter, r *http.Request, room *Room, me, flash string) {
	v := s.buildRoomView(room, me, flash)
	v.Suggestions = s.suggestions(r.Context(), room)
	render(w, http.StatusOK, panelTmpl, "panel", v)
}

func (s *Server) handleRoomPage(w http.ResponseWriter, r *http.Request) {
	u := s.session(w, r)
	room, err := s.store.Get(r.PathValue("code"))
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	me := ""
	if u != nil {
		me = u.ID
		room.Join(u) // a link-follower joins if there is room; otherwise they spectate
	}
	s.renderRoom(w, r, room, me, "")
}

func (s *Server) handlePanel(w http.ResponseWriter, r *http.Request) {
	room, err := s.store.Get(r.PathValue("code"))
	if err != nil {
		http.Error(w, "gone", http.StatusGone)
		return
	}
	me := ""
	if u := s.auth.User(r); u != nil {
		me = u.ID
	}
	s.renderPanel(w, r, room, me, "")
}

func (s *Server) handleRoll(w http.ResponseWriter, r *http.Request) {
	u, room, ok := s.actor(w, r)
	if !ok {
		return
	}
	flash := ""
	if err := room.Roll(u.ID); err != nil {
		flash = "Could not roll right now."
	}
	s.renderPanel(w, r, room, u.ID, flash)
}

func (s *Server) handleTopic(w http.ResponseWriter, r *http.Request) {
	u, room, ok := s.actor(w, r)
	if !ok {
		return
	}
	r.ParseForm()
	topic := strings.TrimSpace(r.FormValue("topic"))
	if topic == "" {
		s.renderPanel(w, r, room, u.ID, "Pick a topic first.")
		return
	}

	room.mu.Lock()
	switch {
	case room.Phase != PhaseTopic:
		room.mu.Unlock()
		s.renderPanel(w, r, room, u.ID, "The topic has already been chosen.")
		return
	case room.TopicPicker() != u.ID:
		room.mu.Unlock()
		s.renderPanel(w, r, room, u.ID, "The dice gave the topic pick to someone else.")
		return
	}
	room.Topic = topic
	room.mu.Unlock()

	subs := s.resolveSubTopics(r.Context(), room, topic)
	if len(subs) == 0 {
		room.mu.Lock()
		room.Topic = ""
		room.mu.Unlock()
		s.renderPanel(w, r, room, u.ID, "Nothing playable for that topic. Try another.")
		return
	}

	room.mu.Lock()
	room.SubTopics = subs
	room.Phase = PhaseSubTopics
	// With two players nobody follows the topic picker, so their own choice
	// stands and the round builds immediately.
	if room.NextSubTopicPicker() == "" {
		room.SubTopics[0].ClaimedBy = u.ID
		room.Phase = PhaseBuilding
	}
	building := room.Phase == PhaseBuilding
	room.mu.Unlock()

	if building {
		go s.startQuiz(room)
	}
	s.renderPanel(w, r, room, u.ID, "")
}

func (s *Server) handleSubTopic(w http.ResponseWriter, r *http.Request) {
	u, room, ok := s.actor(w, r)
	if !ok {
		return
	}
	r.ParseForm()
	key := r.FormValue("key")

	room.mu.Lock()
	if room.Phase != PhaseSubTopics {
		room.mu.Unlock()
		s.renderPanel(w, r, room, u.ID, "")
		return
	}
	if room.NextSubTopicPicker() != u.ID {
		room.mu.Unlock()
		s.renderPanel(w, r, room, u.ID, "Wait for your turn.")
		return
	}

	claimed := false
	for i := range room.SubTopics {
		if room.SubTopics[i].Key != key || room.SubTopics[i].ClaimedBy != "" {
			continue
		}
		room.SubTopics[i].ClaimedBy = u.ID
		if p := room.find(u.ID); p != nil {
			p.SubTopic = key
		}
		claimed = true
		break
	}
	if !claimed {
		room.mu.Unlock()
		s.renderPanel(w, r, room, u.ID, "Someone just took that one.")
		return
	}

	building := room.NextSubTopicPicker() == ""
	if building {
		room.Phase = PhaseBuilding
	}
	room.mu.Unlock()

	if building {
		go s.startQuiz(room)
	}
	s.renderPanel(w, r, room, u.ID, "")
}

func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request) {
	u, room, ok := s.actor(w, r)
	if !ok {
		return
	}
	r.ParseForm()
	index := atoi(r.FormValue("index"), -1)
	choice := atoi(r.FormValue("choice"), -1)

	room.mu.Lock()
	defer room.mu.Unlock()

	if room.Phase != PhaseQuiz || index != room.Current || room.Current >= len(room.Questions) {
		v := s.roomViewLocked(room, u.ID, "")
		render(w, http.StatusOK, panelTmpl, "panel", v)
		return
	}
	me := room.find(u.ID)
	if me == nil || me.answered[index] {
		v := s.roomViewLocked(room, u.ID, "")
		render(w, http.StatusOK, panelTmpl, "panel", v)
		return
	}

	q := room.Questions[room.Current]
	correct := choice == q.Answer
	elapsed := int(time.Since(room.questionAt).Milliseconds())

	points := 0
	if correct {
		// Speed matters, but never more than being right: 100 base, up to 50
		// more for answering inside the first ten seconds.
		points = 100
		if bonus := 50 - elapsed/200; bonus > 0 {
			points += bonus
		}
		// You seeded this axis, so you get less for knowing it.
		if q.SeededBy == u.ID {
			points /= 2
		}
	}
	me.Score += points
	me.answered[index] = true
	q.Answers = append(q.Answers, Answer{PlayerID: u.ID, Choice: choice, Correct: correct, Points: points, Elapsed: elapsed})

	// Render this player's result before advancing, so they see the answer.
	v := s.roomViewLocked(room, u.ID, "")

	if len(q.Answers) >= len(room.Players) {
		if room.Current+1 >= len(room.Questions) {
			room.Phase = PhaseResults
			s.board.Record(room)
		} else {
			room.Current++
			room.questionAt = time.Now()
		}
	}
	render(w, http.StatusOK, panelTmpl, "panel", v)
}

// actor resolves the (player, room) pair every game action needs.
func (s *Server) actor(w http.ResponseWriter, r *http.Request) (*D3bitUser, *Room, bool) {
	u := s.auth.User(r)
	if u == nil {
		s.fail(w, r, "Your session expired. Start again.")
		return nil, nil, false
	}
	room, err := s.store.Get(r.PathValue("code"))
	if err != nil {
		s.fail(w, r, "That room is gone.")
		return nil, nil, false
	}
	return u, room, true
}

func (s *Server) suggestions(ctx context.Context, room *Room) []string {
	if room.Phase != PhaseTopic {
		return nil
	}
	return OfflineTopicSuggestions()
}

func (s *Server) resolveSubTopics(ctx context.Context, room *Room, topic string) []SubTopic {
	if !s.cala.Enabled() {
		return offlineSubTopics(topic)
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	entities, err := s.cala.SearchEntities(ctx, topic, 5)
	if err != nil || len(entities) == 0 {
		return offlineSubTopics(topic)
	}
	room.TopicEntity = entities[0].ID

	in, err := s.cala.Introspect(ctx, entities[0].ID)
	if err != nil {
		return offlineSubTopics(topic)
	}
	if subs := SubTopics(in, 5); len(subs) > 0 {
		return subs
	}
	return offlineSubTopics(topic)
}

// startQuiz builds questions off the request path so the picking player is not
// left waiting on Cala; the room polls its way into the quiz phase.
func (s *Server) startQuiz(room *Room) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	qs, err := s.buildQuestions(ctx, room)
	if err == nil {
		s.fal.AddQuestionImages(ctx, room.Topic, qs, 2)
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	if err != nil || len(qs) == 0 {
		room.Error = "Could not build a round from that topic. Start another game."
		room.Phase = PhaseResults
		return
	}
	room.Questions = qs
	room.QuestionCount = len(qs) // the built count, not the requested one
	room.Current = 0
	room.questionAt = time.Now()
	room.Phase = PhaseQuiz
}

func urlQueryEscape(s string) string { return url.QueryEscape(s) }
func urlPathEscape(s string) string  { return url.PathEscape(s) }
func lower(s string) string          { return strings.ToLower(s) }

func atoi(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}
