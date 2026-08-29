package main

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"sync"
	"time"
)

// Phase is the room's position in the game loop.
type Phase string

const (
	PhaseLobby     Phase = "lobby"     // waiting for players
	PhaseRolling   Phase = "rolling"   // dice decide turn order
	PhaseTopic     Phase = "topic"     // first player picks the overall entity topic
	PhaseSubTopics Phase = "subtopics" // every other player claims one of the top 5 axes
	PhaseBuilding  Phase = "building"  // questions are being assembled from Cala
	PhaseQuiz      Phase = "quiz"
	PhaseResults   Phase = "results"
)

type SubTopicKind string

const (
	SubTopicNumeric  SubTopicKind = "numeric"
	SubTopicRelation SubTopicKind = "relation"
	SubTopicProperty SubTopicKind = "property"
	SubTopicMetric   SubTopicKind = "metric" // a time series (revenue, headcount…); latest point is used
)

type SubTopic struct {
	Key   string       `json:"key"`
	Label string       `json:"label"`
	Kind  SubTopicKind `json:"kind"`

	// ClaimedBy is the player id that picked this axis, empty while free.
	ClaimedBy string `json:"claimed_by,omitempty"`
}

type Player struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Color       string `json:"color"`
	IsAnon      bool   `json:"is_anon"`
	IsHost      bool   `json:"is_host"`

	Roll  int `json:"roll"` // dice result, 0 until rolled
	Score int `json:"score"`

	// SubTopic is the axis this player claimed; the first player picks the
	// root topic instead and leaves this empty.
	SubTopic string `json:"sub_topic,omitempty"`

	joinedAt time.Time
	answered map[int]bool
}

type Answer struct {
	PlayerID string `json:"player_id"`
	Choice   int    `json:"choice"`
	Correct  bool   `json:"correct"`
	Points   int    `json:"points"`
	Elapsed  int    `json:"elapsed_ms"`
}

type Question struct {
	Index     int      `json:"index"`
	Kind      string   `json:"kind"`
	Prompt    string   `json:"prompt"`
	Options   []string `json:"options"`
	Answer    int      `json:"-"` // never serialized while the question is live
	Fact      string   `json:"fact,omitempty"`
	Source    string   `json:"source,omitempty"`
	SourceURL string   `json:"source_url,omitempty"`

	// SeededBy is the player whose sub-topic produced this question. They
	// score less on it — knowing your own axis shouldn't win the game.
	SeededBy string `json:"seeded_by,omitempty"`

	Answers []Answer `json:"-"`
}

type Room struct {
	Code      string    `json:"code"`
	Public    bool      `json:"public"`
	Phase     Phase     `json:"phase"`
	MaxPlayer int       `json:"max_players"`
	Players   []*Player `json:"players"`

	// Order holds player ids sorted by dice roll, highest first.
	Order []string `json:"order"`

	Topic       string     `json:"topic"`
	TopicEntity string     `json:"topic_entity,omitempty"`
	SubTopics   []SubTopic `json:"sub_topics"`

	// graph is what Cala resolved for Topic; nil in offline mode.
	graph *TopicGraph

	Questions []*Question `json:"-"`
	Current   int         `json:"current"`

	QuestionCount int    `json:"question_count"`
	Error         string `json:"error,omitempty"`
	// Status is what the room is doing while players wait (resolving a
	// topic, pulling values); Notice is a one-off message for the next screen.
	Status string `json:"status,omitempty"`
	Notice string `json:"notice,omitempty"`

	CreatedAt  time.Time `json:"created_at"`
	questionAt time.Time
	mu         sync.Mutex
}

var (
	ErrRoomNotFound = errors.New("room not found")
	ErrRoomFull     = errors.New("room is full")
	ErrNotYourTurn  = errors.New("not your turn")
	ErrBadPhase     = errors.New("wrong phase for that action")
	ErrTaken        = errors.New("already taken")
)

// Store keeps rooms in memory. Iteration 0 deliberately has no database:
// rooms are ephemeral, a hackathon demo never outlives the process, and
// skipping persistence removes migrations from the critical path.
type Store struct {
	mu    sync.RWMutex
	rooms map[string]*Room
}

func NewStore() *Store {
	s := &Store{rooms: make(map[string]*Room)}
	go s.reap()
	return s
}

// reap drops rooms nobody has touched for an hour so a long-running server
// doesn't leak abandoned lobbies.
func (s *Store) reap() {
	for range time.Tick(10 * time.Minute) {
		cutoff := time.Now().Add(-1 * time.Hour)
		s.mu.Lock()
		for code, r := range s.rooms {
			if r.CreatedAt.Before(cutoff) {
				delete(s.rooms, code)
			}
		}
		s.mu.Unlock()
	}
}

func (s *Store) Create(public bool, maxPlayers, questions int) *Room {
	if maxPlayers < 2 {
		maxPlayers = 2
	}
	if maxPlayers > 4 {
		maxPlayers = 4
	}
	if questions <= 0 {
		questions = 8
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var code string
	for {
		code = newCode()
		if _, clash := s.rooms[code]; !clash {
			break
		}
	}

	r := &Room{
		Code:          code,
		Public:        public,
		Phase:         PhaseLobby,
		MaxPlayer:     maxPlayers,
		QuestionCount: questions,
		CreatedAt:     time.Now(),
	}
	s.rooms[code] = r
	return r
}

func (s *Store) Get(code string) (*Room, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.rooms[strings.ToUpper(strings.TrimSpace(code))]
	if !ok {
		return nil, ErrRoomNotFound
	}
	return r, nil
}

// PublicRooms lists joinable public lobbies, newest first.
func (s *Store) PublicRooms() []*Room {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*Room
	for _, r := range s.rooms {
		r.mu.Lock()
		joinable := r.Public && r.Phase == PhaseLobby && len(r.Players) < r.MaxPlayer
		r.mu.Unlock()
		if joinable {
			out = append(out, r)
		}
	}
	return out
}

// Join adds a player, or returns the existing one on reconnect.
func (r *Room) Join(u *D3bitUser) (*Player, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range r.Players {
		if p.ID == u.ID {
			return p, nil // rejoin is idempotent
		}
	}
	if r.Phase != PhaseLobby {
		return nil, ErrBadPhase
	}
	if len(r.Players) >= r.MaxPlayer {
		return nil, ErrRoomFull
	}

	p := &Player{
		ID:          u.ID,
		DisplayName: firstNonEmpty(u.DisplayName, u.Name, u.Username, "Player"),
		Color:       firstNonEmpty(u.Color, "#7c9cff"),
		IsAnon:      u.IsAnon,
		IsHost:      len(r.Players) == 0,
		joinedAt:    time.Now(),
		answered:    make(map[int]bool),
	}
	r.Players = append(r.Players, p)

	// A full room rolls immediately — nobody should have to press "start".
	if len(r.Players) == r.MaxPlayer {
		r.Phase = PhaseRolling
	}
	return p, nil
}

// Roll assigns this player a die. Once everyone has rolled, the order is
// locked in highest-first and the first player moves on to pick the topic.
func (r *Room) Roll(playerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Phase != PhaseRolling {
		return ErrBadPhase
	}
	me := r.find(playerID)
	if me == nil {
		return ErrRoomNotFound
	}
	if me.Roll != 0 {
		return nil // already rolled; keep it idempotent
	}
	me.Roll = 1 + randInt(6)

	for _, p := range r.Players {
		if p.Roll == 0 {
			return nil // still waiting on someone
		}
	}

	// Ties break on join order, so the result is always a strict ordering.
	ordered := append([]*Player{}, r.Players...)
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			a, b := ordered[i], ordered[j]
			if b.Roll > a.Roll || (b.Roll == a.Roll && b.joinedAt.Before(a.joinedAt)) {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}
	r.Order = nil
	for _, p := range ordered {
		r.Order = append(r.Order, p.ID)
	}
	r.Phase = PhaseTopic
	return nil
}

// TopicPicker is the player who won the roll.
func (r *Room) TopicPicker() string {
	if len(r.Order) == 0 {
		return ""
	}
	return r.Order[0]
}

// NextSubTopicPicker returns whoever is on the clock, in roll order after the
// topic picker, or "" when everyone has claimed an axis.
func (r *Room) NextSubTopicPicker() string {
	if len(r.Order) < 2 {
		return "" // order isn't decided yet, or nobody follows the topic picker
	}
	for _, id := range r.Order[1:] {
		if p := r.find(id); p != nil && p.SubTopic == "" {
			return id
		}
	}
	return ""
}

func (r *Room) find(id string) *Player {
	for _, p := range r.Players {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// newCode builds a 4-character room code from an alphabet with no 0/O or 1/I,
// so a code read aloud across a noisy room still gets typed correctly.
func newCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 4)
	for i := range b {
		b[i] = alphabet[randInt(len(alphabet))]
	}
	return string(b)
}

func randInt(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}
