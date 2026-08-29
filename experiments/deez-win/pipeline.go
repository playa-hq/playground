package main

import "time"

// A LoadStep is one piece of background work the room is waiting on: the
// topic resolving, an axis's values arriving, cover art rendering. Steps are
// upserted by key so a goroutine can report progress with one call, and the
// roster panel draws them as the round's loading graph.
type LoadStep struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	State  string `json:"state"` // pending · running · done · failed
	Detail string `json:"detail,omitempty"`
	Done   int    `json:"done,omitempty"`
	Total  int    `json:"total,omitempty"`

	startedAt time.Time
	endedAt   time.Time
}

const (
	StepPending = "pending"
	StepRunning = "running"
	StepDone    = "done"
	StepFailed  = "failed"
)

// progress upserts a step. It takes the room lock, so call it from
// background work, never while holding the lock.
func (r *Room) progress(key, label, state, detail string, done, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var st *LoadStep
	for _, s := range r.Loads {
		if s.Key == key {
			st = s
			break
		}
	}
	if st == nil {
		st = &LoadStep{Key: key, startedAt: time.Now()}
		r.Loads = append(r.Loads, st)
	}
	if label != "" {
		st.Label = label
	}
	st.State, st.Detail, st.Done, st.Total = state, detail, done, total
	if state == StepDone || state == StepFailed {
		st.endedAt = time.Now()
	}
}

// Elapsed is how long the step took, or has been running.
func (s *LoadStep) Elapsed() string {
	end := s.endedAt
	if end.IsZero() {
		if s.State != StepRunning {
			return ""
		}
		end = time.Now()
	}
	d := end.Sub(s.startedAt).Round(100 * time.Millisecond)
	if d < time.Second {
		return "<1s"
	}
	return d.Round(time.Second).String()
}

// Pct is the step's progress for the bar, 0–100.
func (s *LoadStep) Pct() int {
	if s.State == StepDone {
		return 100
	}
	if s.Total == 0 {
		return 0
	}
	return s.Done * 100 / s.Total
}
