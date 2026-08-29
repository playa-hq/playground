package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// sessionSource is whatever can resolve a request to a player identity:
// the real D3BIT proxy, or the local dev stand-in.
type sessionSource interface {
	Routes(mux *http.ServeMux)
	User(r *http.Request) *D3bitUser
}

type Server struct {
	auth     sessionSource
	d3bitURL string
	store    *Store
	cala     *Cala
}

func (s *Server) Routes(mux *http.ServeMux) {
	s.auth.Routes(mux)

	mux.HandleFunc("GET /api/config", s.handleConfig)
	mux.HandleFunc("GET /api/rooms", s.handlePublicRooms)
	mux.HandleFunc("POST /api/rooms", s.handleCreateRoom)
	mux.HandleFunc("POST /api/rooms/{code}/join", s.handleJoin)
	mux.HandleFunc("GET /api/rooms/{code}", s.handleRoomState)
	mux.HandleFunc("POST /api/rooms/{code}/roll", s.handleRoll)
	mux.HandleFunc("GET /api/rooms/{code}/topics", s.handleTopicSuggestions)
	mux.HandleFunc("POST /api/rooms/{code}/topic", s.handlePickTopic)
	mux.HandleFunc("POST /api/rooms/{code}/subtopic", s.handlePickSubTopic)
	mux.HandleFunc("POST /api/rooms/{code}/answer", s.handleAnswer)
}

// requireUser resolves the D3BIT session. The browser is expected to have
// called POST /d3bit/auth/anon first, so an unauthenticated request here is a
// client bug rather than a normal state.
func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) *D3bitUser {
	u := s.auth.User(r)
	if u == nil {
		httpError(w, http.StatusUnauthorized, "unauthenticated", "No D3BIT session. Create one first.")
		return nil
	}
	return u
}

func (s *Server) room(w http.ResponseWriter, r *http.Request) *Room {
	room, err := s.store.Get(r.PathValue("code"))
	if err != nil {
		httpError(w, http.StatusNotFound, "room_not_found", "No room with that code.")
		return nil
	}
	return room
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"cala_enabled": s.cala.Enabled(),
		"d3bit_url":    s.d3bitURL,
	})
}

func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}

	var body struct {
		Public    bool `json:"public"`
		MaxPlayer int  `json:"max_players"`
		Questions int  `json:"questions"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	room := s.store.Create(body.Public, body.MaxPlayer, body.Questions)
	if _, err := room.Join(u); err != nil {
		httpError(w, http.StatusConflict, "join_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.view(room, u.ID))
}

func (s *Server) handlePublicRooms(w http.ResponseWriter, r *http.Request) {
	rooms := s.store.PublicRooms()
	sort.Slice(rooms, func(i, j int) bool { return rooms[i].CreatedAt.After(rooms[j].CreatedAt) })

	out := make([]map[string]any, 0, len(rooms))
	for _, room := range rooms {
		room.mu.Lock()
		out = append(out, map[string]any{
			"code":        room.Code,
			"players":     len(room.Players),
			"max_players": room.MaxPlayer,
		})
		room.mu.Unlock()
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": out})
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	room := s.room(w, r)
	if room == nil {
		return
	}

	if _, err := room.Join(u); err != nil {
		status := http.StatusConflict
		if err == ErrRoomFull {
			status = http.StatusForbidden
		}
		httpError(w, status, "join_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.view(room, u.ID))
}

func (s *Server) handleRoomState(w http.ResponseWriter, r *http.Request) {
	room := s.room(w, r)
	if room == nil {
		return
	}
	me := ""
	if u := s.auth.User(r); u != nil {
		me = u.ID
	}
	writeJSON(w, http.StatusOK, s.view(room, me))
}

func (s *Server) handleRoll(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	room := s.room(w, r)
	if room == nil {
		return
	}
	if err := room.Roll(u.ID); err != nil {
		httpError(w, http.StatusConflict, "roll_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.view(room, u.ID))
}

// handleTopicSuggestions offers starting points for the first player.
func (s *Server) handleTopicSuggestions(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if !s.cala.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"topics": OfflineTopicSuggestions(), "offline": true})
		return
	}
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"topics": OfflineTopicSuggestions()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	entities, err := s.cala.SearchEntities(ctx, q, 6)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"topics": []string{q}, "degraded": true})
		return
	}
	topics := make([]string, 0, len(entities))
	for _, e := range entities {
		topics = append(topics, e.Name)
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": topics})
}

// handlePickTopic locks the overall topic and resolves the top five sub-topics
// that the remaining players will claim.
func (s *Server) handlePickTopic(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	room := s.room(w, r)
	if room == nil {
		return
	}

	var body struct {
		Topic string `json:"topic"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	topic := strings.TrimSpace(body.Topic)
	if topic == "" {
		httpError(w, http.StatusBadRequest, "missing_topic", "Pick a topic first.")
		return
	}

	room.mu.Lock()
	if room.Phase != PhaseTopic {
		room.mu.Unlock()
		httpError(w, http.StatusConflict, "bad_phase", "The topic has already been chosen.")
		return
	}
	if room.TopicPicker() != u.ID {
		room.mu.Unlock()
		httpError(w, http.StatusForbidden, "not_your_turn", "The dice gave the topic pick to someone else.")
		return
	}
	room.Topic = topic
	room.mu.Unlock()

	subs := s.resolveSubTopics(r.Context(), room, topic)
	if len(subs) == 0 {
		httpError(w, http.StatusUnprocessableEntity, "no_data",
			"Cala has nothing playable for that topic. Try another.")
		return
	}

	room.mu.Lock()
	room.SubTopics = subs
	room.Phase = PhaseSubTopics
	// With two players there is no one left to claim an axis, so the topic
	// picker's own choice stands and we go straight to building.
	if room.NextSubTopicPicker() == "" {
		room.SubTopics[0].ClaimedBy = u.ID
		room.Phase = PhaseBuilding
	}
	room.mu.Unlock()

	if room.Phase == PhaseBuilding {
		go s.startQuiz(room)
	}
	writeJSON(w, http.StatusOK, s.view(room, u.ID))
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
	subs := SubTopics(in, 5)
	if len(subs) == 0 {
		return offlineSubTopics(topic)
	}
	return subs
}

func (s *Server) handlePickSubTopic(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	room := s.room(w, r)
	if room == nil {
		return
	}

	var body struct {
		Key string `json:"key"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	room.mu.Lock()
	if room.Phase != PhaseSubTopics {
		room.mu.Unlock()
		httpError(w, http.StatusConflict, "bad_phase", "Not picking sub-topics right now.")
		return
	}
	if room.NextSubTopicPicker() != u.ID {
		room.mu.Unlock()
		httpError(w, http.StatusForbidden, "not_your_turn", "Wait for your turn.")
		return
	}

	claimed := false
	for i := range room.SubTopics {
		if room.SubTopics[i].Key != body.Key {
			continue
		}
		if room.SubTopics[i].ClaimedBy != "" {
			room.mu.Unlock()
			httpError(w, http.StatusConflict, "taken", "Someone just took that one.")
			return
		}
		room.SubTopics[i].ClaimedBy = u.ID
		if p := room.find(u.ID); p != nil {
			p.SubTopic = body.Key
		}
		claimed = true
		break
	}
	if !claimed {
		room.mu.Unlock()
		httpError(w, http.StatusBadRequest, "unknown_subtopic", "That sub-topic is not on the board.")
		return
	}

	done := room.NextSubTopicPicker() == ""
	if done {
		room.Phase = PhaseBuilding
	}
	room.mu.Unlock()

	if done {
		go s.startQuiz(room)
	}
	writeJSON(w, http.StatusOK, s.view(room, u.ID))
}

// startQuiz builds the questions off the request path so the picking player
// isn't left waiting on Cala; the room polls its way into the quiz phase.
func (s *Server) startQuiz(room *Room) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	qs, err := s.buildQuestions(ctx, room)

	room.mu.Lock()
	defer room.mu.Unlock()

	if err != nil || len(qs) == 0 {
		room.Error = "Could not build a round from that topic. Start another game."
		room.Phase = PhaseResults
		return
	}
	room.Questions = qs
	room.Current = 0
	room.questionAt = time.Now()
	room.Phase = PhaseQuiz
}

func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	room := s.room(w, r)
	if room == nil {
		return
	}

	var body struct {
		Index  int `json:"index"`
		Choice int `json:"choice"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	room.mu.Lock()
	defer room.mu.Unlock()

	if room.Phase != PhaseQuiz {
		httpError(w, http.StatusConflict, "bad_phase", "No question is live.")
		return
	}
	if body.Index != room.Current || room.Current >= len(room.Questions) {
		httpError(w, http.StatusConflict, "stale_answer", "That question has moved on.")
		return
	}
	me := room.find(u.ID)
	if me == nil {
		httpError(w, http.StatusForbidden, "not_in_room", "You are not in this room.")
		return
	}
	if me.answered[body.Index] {
		writeJSON(w, http.StatusOK, s.viewLocked(room, u.ID))
		return
	}

	q := room.Questions[room.Current]
	correct := body.Choice == q.Answer
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
			points = points / 2
		}
	}
	me.Score += points
	me.answered[body.Index] = true
	q.Answers = append(q.Answers, Answer{
		PlayerID: u.ID, Choice: body.Choice, Correct: correct, Points: points, Elapsed: elapsed,
	})

	// Advance as soon as everyone has answered.
	if len(q.Answers) >= len(room.Players) {
		if room.Current+1 >= len(room.Questions) {
			room.Phase = PhaseResults
		} else {
			room.Current++
			room.questionAt = time.Now()
		}
	}
	writeJSON(w, http.StatusOK, s.viewLocked(room, u.ID))
}

// view is the room as one player should see it.
func (s *Server) view(room *Room, me string) map[string]any {
	room.mu.Lock()
	defer room.mu.Unlock()
	return s.viewLocked(room, me)
}

// viewLocked builds the client payload. The correct answer is withheld until
// the question is settled — the client is never trusted with it early.
func (s *Server) viewLocked(room *Room, me string) map[string]any {
	out := map[string]any{
		"code":           room.Code,
		"public":         room.Public,
		"phase":          room.Phase,
		"max_players":    room.MaxPlayer,
		"players":        room.Players,
		"order":          room.Order,
		"topic":          room.Topic,
		"sub_topics":     room.SubTopics,
		"question_count": len(room.Questions),
		"current":        room.Current,
		"me":             me,
		"error":          room.Error,
		"topic_picker":   room.TopicPicker(),
		"next_picker":    room.NextSubTopicPicker(),
		"cala_enabled":   s.cala.Enabled(),
	}

	if room.Phase == PhaseQuiz && room.Current < len(room.Questions) {
		q := room.Questions[room.Current]
		answered := false
		if p := room.find(me); p != nil {
			answered = p.answered[q.Index]
		}

		payload := map[string]any{
			"index":     q.Index,
			"kind":      q.Kind,
			"prompt":    q.Prompt,
			"options":   q.Options,
			"seeded_by": q.SeededBy,
			"answered":  answered,
			"waiting":   len(room.Players) - len(q.Answers),
		}
		// Only reveal the answer to a player who has already committed.
		if answered {
			payload["answer"] = q.Answer
			payload["fact"] = q.Fact
			payload["source"] = q.Source
			payload["source_url"] = q.SourceURL
		}
		out["question"] = payload
	}

	if room.Phase == PhaseResults {
		var review []map[string]any
		for _, q := range room.Questions {
			review = append(review, map[string]any{
				"prompt": q.Prompt, "options": q.Options, "answer": q.Answer,
				"fact": q.Fact, "source": q.Source, "source_url": q.SourceURL,
			})
		}
		out["review"] = review
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
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
