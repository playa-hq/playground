package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// The leaderboard is the one thing on the original sketch that the game never
// had: rooms are ephemeral, so a score vanished the moment a round ended and
// there was nothing to come back for.
//
// It persists to a JSON file rather than a database. Rooms stay in memory —
// they are throwaway — but a score is the only thing worth surviving a deploy,
// and one file with a mutex is the whole implementation.

type Standing struct {
	PlayerID    string    `json:"player_id"`
	DisplayName string    `json:"display_name"`
	Color       string    `json:"color"`
	Score       int       `json:"score"`
	Games       int       `json:"games"`
	Wins        int       `json:"wins"`
	LastSeen    time.Time `json:"last_seen"`
}

type Leaderboard struct {
	path string

	mu       sync.RWMutex
	byPlayer map[string]*Standing
	dirty    bool
}

func NewLeaderboard(path string) *Leaderboard {
	l := &Leaderboard{path: path, byPlayer: make(map[string]*Standing)}
	l.load()
	// Flush on a timer instead of on every write: a round ends in a burst of
	// updates, and losing a few seconds of scores is not worth an fsync each.
	go l.flushLoop()
	return l
}

func (l *Leaderboard) load() {
	if l.path == "" {
		return
	}
	raw, err := os.ReadFile(l.path)
	if err != nil {
		return // first run
	}
	var rows []*Standing
	if json.Unmarshal(raw, &rows) != nil {
		return // corrupt file: start clean rather than refuse to boot
	}
	for _, r := range rows {
		l.byPlayer[r.PlayerID] = r
	}
}

func (l *Leaderboard) flushLoop() {
	for range time.Tick(15 * time.Second) {
		l.Flush()
	}
}

// Flush writes through a temp file so a crash mid-write cannot truncate the
// existing standings.
func (l *Leaderboard) Flush() {
	if l.path == "" {
		return
	}
	l.mu.Lock()
	if !l.dirty {
		l.mu.Unlock()
		return
	}
	rows := make([]*Standing, 0, len(l.byPlayer))
	for _, r := range l.byPlayer {
		rows = append(rows, r)
	}
	l.dirty = false
	l.mu.Unlock()

	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return
	}
	tmp := l.path + ".tmp"
	if os.WriteFile(tmp, raw, 0o644) != nil {
		return
	}
	os.Rename(tmp, l.path)
}

// Record folds one finished room into the standings.
func (l *Leaderboard) Record(room *Room) {
	if len(room.Players) == 0 {
		return
	}

	best := room.Players[0].Score
	for _, p := range room.Players[1:] {
		if p.Score > best {
			best = p.Score
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	for _, p := range room.Players {
		s, ok := l.byPlayer[p.ID]
		if !ok {
			s = &Standing{PlayerID: p.ID}
			l.byPlayer[p.ID] = s
		}
		s.DisplayName = p.DisplayName
		s.Color = p.Color
		s.Score += p.Score
		s.Games++
		// A shared top score counts for everyone who reached it: with 2-4
		// players, ties are common enough that breaking them arbitrarily reads
		// as a bug.
		if p.Score == best {
			s.Wins++
		}
		s.LastSeen = time.Now()
	}
	l.dirty = true
}

// Top returns the highest scorers, best first.
func (l *Leaderboard) Top(n int) []*Standing {
	l.mu.RLock()
	defer l.mu.RUnlock()

	out := make([]*Standing, 0, len(l.byPlayer))
	for _, s := range l.byPlayer {
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Wins > out[j].Wins
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// Rank is a player's 1-based position, or 0 when they have never played.
func (l *Leaderboard) Rank(playerID string) int {
	for i, s := range l.Top(0) {
		if s.PlayerID == playerID {
			return i + 1
		}
	}
	return 0
}

func defaultLeaderboardPath() string {
	if dir := os.Getenv("DEEZ_STATE_DIR"); dir != "" {
		return filepath.Join(dir, "leaderboard.json")
	}
	return "leaderboard.json"
}
