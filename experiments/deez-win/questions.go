package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// buildQuestions assembles the round from the axes players claimed.
//
// The rule that keeps this honest: Cala decides what is true, the model only
// decides how it reads. A numeric axis becomes a higher/lower question with a
// value straight off the graph — no model in the loop at all, so there is
// nothing to hallucinate. Anything we cannot ground, we drop.
func (s *Server) buildQuestions(ctx context.Context, r *Room) ([]*Question, error) {
	if !s.cala.Enabled() {
		return offlineQuestions(r), nil
	}

	var qs []*Question
	perAxis := r.QuestionCount / max(1, len(r.SubTopicsClaimed()))

	for _, st := range r.SubTopicsClaimed() {
		entities, err := s.cala.SearchEntities(ctx, r.Topic, 12)
		if err != nil || len(entities) < 2 {
			continue
		}

		switch st.Kind {
		case SubTopicNumeric:
			qs = append(qs, s.numericQuestions(ctx, r, st, entities, perAxis)...)
		default:
			qs = append(qs, s.factQuestions(ctx, r, st, entities, perAxis)...)
		}
	}

	// A round with no grounded questions is a failed round. Say so rather
	// than papering over it with invented content.
	if len(qs) == 0 {
		return offlineQuestions(r), nil
	}

	for i, q := range qs {
		q.Index = i
	}
	if len(qs) > r.QuestionCount {
		qs = qs[:r.QuestionCount]
	}
	return qs, nil
}

// numericQuestions builds higher/lower pairs from a numerical observation.
func (s *Server) numericQuestions(ctx context.Context, r *Room, st SubTopic, entities []CalaEntity, n int) []*Question {
	type valued struct {
		name string
		val  float64
		src  CalaSource
	}

	var pool []valued
	for _, e := range entities {
		results, _, err := s.cala.Query(ctx, fmt.Sprintf("%s.%s", e.Name, st.Key))
		if err != nil || len(results) == 0 {
			continue
		}
		v, src, ok := firstNumber(results[0])
		if !ok {
			continue
		}
		pool = append(pool, valued{name: e.Name, val: v, src: src})
	}
	if len(pool) < 2 {
		return nil
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].val < pool[j].val })

	var out []*Question
	for i := 0; i+1 < len(pool) && len(out) < n; i += 2 {
		a, b := pool[i], pool[i+1]
		if a.val == b.val {
			continue // no defensible answer
		}
		prompt, lowerWins := numericPrompt(st.Label, st.Key)
		// The pool is sorted ascending, so b is the larger of the pair.
		answer, src := 1, b.src
		if lowerWins {
			answer, src = 0, a.src
		}
		out = append(out, &Question{
			Kind:      "higher_lower",
			Prompt:    prompt,
			Options:   []string{a.name, b.name},
			Answer:    answer,
			Fact:      fmt.Sprintf("%s: %s — %s: %s", a.name, formatValue(st.Key, a.val), b.name, formatValue(st.Key, b.val)),
			Source:    src.Name,
			SourceURL: src.URL,
			SeededBy:  st.ClaimedBy,
		})
	}
	return out
}

// factQuestions builds "which one is it" questions from a relationship or
// property, using other entities in the same set as distractors.
func (s *Server) factQuestions(ctx context.Context, r *Room, st SubTopic, entities []CalaEntity, n int) []*Question {
	var out []*Question

	for _, e := range entities {
		if len(out) >= n {
			break
		}
		results, _, err := s.cala.Query(ctx, fmt.Sprintf("%s.%s", e.Name, st.Key))
		if err != nil || len(results) == 0 {
			continue
		}
		answer, src, ok := firstString(results[0])
		if !ok || answer == "" {
			continue
		}

		options := []string{answer}
		for _, other := range entities {
			if len(options) >= 4 {
				break
			}
			if other.ID == e.ID || other.Name == answer {
				continue
			}
			options = append(options, other.Name)
		}
		if len(options) < 3 {
			continue
		}

		correct := randInt(len(options))
		options[0], options[correct] = options[correct], options[0]

		out = append(out, &Question{
			Kind:      "multiple_choice",
			Prompt:    fmt.Sprintf("%s — %s?", e.Name, strings.ToLower(st.Label)),
			Options:   options,
			Answer:    correct,
			Fact:      fmt.Sprintf("%s %s: %s", e.Name, strings.ToLower(st.Label), answer),
			Source:    src.Name,
			SourceURL: src.URL,
			SeededBy:  st.ClaimedBy,
		})
	}
	return out
}

// SubTopicsClaimed returns only the axes players actually picked.
func (r *Room) SubTopicsClaimed() []SubTopic {
	var out []SubTopic
	for _, st := range r.SubTopics {
		if st.ClaimedBy != "" {
			out = append(out, st)
		}
	}
	return out
}

func firstNumber(row map[string]any) (float64, CalaSource, bool) {
	src := sourceOf(row)
	for _, k := range sortedKeys(row) {
		switch v := row[k].(type) {
		case float64:
			return v, src, true
		case int:
			return float64(v), src, true
		}
	}
	return 0, src, false
}

func firstString(row map[string]any) (string, CalaSource, bool) {
	src := sourceOf(row)
	for _, k := range sortedKeys(row) {
		if k == "source" || k == "sources" || k == "origins" {
			continue
		}
		if v, ok := row[k].(string); ok && v != "" {
			return v, src, true
		}
	}
	return "", src, false
}

// sourceOf digs the first citation out of a result row so every question can
// show where its answer came from.
func sourceOf(row map[string]any) CalaSource {
	for _, key := range []string{"origins", "sources", "source"} {
		raw, ok := row[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case map[string]any:
			return calaSourceFromMap(v)
		case []any:
			if len(v) > 0 {
				if m, ok := v[0].(map[string]any); ok {
					return calaSourceFromMap(m)
				}
			}
		}
	}
	return CalaSource{}
}

func calaSourceFromMap(m map[string]any) CalaSource {
	var s CalaSource
	if v, ok := m["name"].(string); ok {
		s.Name = v
	}
	if v, ok := m["url"].(string); ok {
		s.URL = v
	}
	if s.Name == "" && s.URL != "" {
		s.Name = s.URL
	}
	return s
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func formatNum(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.2f", v)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
