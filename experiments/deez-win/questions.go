package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// buildQuestions assembles the round from the axes players claimed.
//
// The rule that keeps this honest: Cala decides what is true, the model only
// decides how it reads. A value comes straight off the graph with its source;
// the code only chooses which two entities to compare. Anything we cannot
// ground, we drop.
func (s *Server) buildQuestions(ctx context.Context, r *Room) ([]*Question, error) {
	if !s.cala.Enabled() || r.graph == nil {
		return offlineQuestions(r), nil
	}

	claimed := r.SubTopicsClaimed()
	if len(claimed) == 0 {
		return nil, fmt.Errorf("no axes claimed")
	}

	r.graph.Fetch(ctx, s.cala, claimed, nil) // no-op for axes already prefetched on claim
	details := r.graph.Details()
	if len(details) < 2 {
		return nil, fmt.Errorf("cala returned too few entities for %q", r.Topic)
	}

	perAxis := max(1, r.QuestionCount/len(claimed))
	var qs []*Question
	for _, st := range claimed {
		qs = append(qs, axisQuestions(st, details, perAxis)...)
	}
	if len(qs) > r.QuestionCount {
		qs = qs[:r.QuestionCount]
	}
	for i, q := range qs {
		q.Index = i
	}
	return qs, nil
}

// grounded is one entity's value on one axis.
type grounded struct {
	name string
	num  float64
	str  string
	unit string
	src  CalaSource
}

// valuesFor reads one axis across the entities, deciding per value whether it
// is a number. A property key that *looks* numeric but holds text becomes a
// fact question rather than a broken comparison.
func valuesFor(st SubTopic, details []*CalaEntityDetail) (nums, strs []grounded) {
	for _, d := range details {
		var v CalaValue
		var ok bool
		switch st.Kind {
		case SubTopicRelation:
			v, ok = d.Relations[st.Key]
		case SubTopicMetric:
			v, ok = d.Metrics[st.Key]
		default:
			v, ok = d.Properties[st.Key]
		}
		if !ok {
			continue
		}
		g := grounded{name: d.Name, src: v.Source, unit: d.Units[st.Key]}
		if n, ok := asNumber(v.Value); ok {
			g.num = n
			nums = append(nums, g)
			continue
		}
		if year, ok := asYear(v.Value); ok {
			g.num = year
			nums = append(nums, g)
			continue
		}
		if s, ok := v.Value.(string); ok && strings.TrimSpace(s) != "" {
			g.str = readable(strings.TrimSpace(s))
			strs = append(strs, g)
		}
	}
	return nums, strs
}

func axisQuestions(st SubTopic, details []*CalaEntityDetail, n int) []*Question {
	nums, strs := valuesFor(st, details)
	if len(nums) >= 2 && len(nums) >= len(strs) {
		return numericQuestions(st, nums, n)
	}
	return factQuestions(st, strs, n)
}

// numericQuestions builds higher/lower pairs. Neighbours in sorted order make
// the closest, hardest pairs; shuffling the pair hides which side is bigger.
func numericQuestions(st SubTopic, pool []grounded, n int) []*Question {
	sort.Slice(pool, func(i, j int) bool { return pool[i].num < pool[j].num })
	prompt, lowerWins := numericPrompt(st.Label, st.Key)

	var out []*Question
	for i := 0; i+1 < len(pool) && len(out) < n; i += 2 {
		a, b := pool[i], pool[i+1]
		if a.num == b.num {
			continue // no defensible answer
		}
		answer, src := 1, b.src // b is the larger of the pair
		if lowerWins {
			answer, src = 0, a.src
		}
		opts := []string{a.name, b.name}
		if randInt(2) == 1 {
			opts[0], opts[1] = opts[1], opts[0]
			answer = 1 - answer
		}
		out = append(out, &Question{
			Kind:      "higher_lower",
			Prompt:    prompt,
			Options:   opts,
			Answer:    answer,
			Fact:      fmt.Sprintf("%s: %s · %s: %s", a.name, formatUnit(st.Key, a.num, a.unit), b.name, formatUnit(st.Key, b.num, b.unit)),
			Source:    src.Name,
			SourceURL: src.URL,
			SeededBy:  st.ClaimedBy,
		})
	}
	return out
}

// factQuestions asks "which is it" with the other entities' values as
// distractors, so every option is a real answer to the same question.
// Two distinct values is the floor — a fair coin, still a fact.
func factQuestions(st SubTopic, pool []grounded, n int) []*Question {
	var out []*Question
	for _, g := range pool {
		if len(out) >= n {
			break
		}
		options := []string{g.str}
		for _, other := range pool {
			if len(options) >= 4 {
				break
			}
			if other.str != g.str && !contains(options, other.str) {
				options = append(options, other.str)
			}
		}
		// Two real values still make a fair question; one does not.
		if len(options) < 2 {
			continue
		}
		correct := randInt(len(options))
		options[0], options[correct] = options[correct], options[0]

		out = append(out, &Question{
			Kind:      "multiple_choice",
			Prompt:    fmt.Sprintf("%s — %s?", g.name, strings.ToLower(st.Label)),
			Options:   options,
			Answer:    correct,
			Fact:      fmt.Sprintf("%s · %s: %s", g.name, strings.ToLower(st.Label), g.str),
			Source:    g.src.Name,
			SourceURL: g.src.URL,
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

func asNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(strings.ReplaceAll(n, ",", ""), 64)
		return f, err == nil
	}
	return 0, false
}

// asYear reads "1977-04-01", "1977-04" or "1977" as a year.
func asYear(v any) (float64, bool) {
	s, ok := v.(string)
	if !ok || len(s) < 4 {
		return 0, false
	}
	if len(s) > 4 && s[4] != '-' {
		return 0, false
	}
	y, err := strconv.Atoi(s[:4])
	if err != nil {
		return 0, false
	}
	return float64(y), true
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

// readable turns taxonomy codes like FINANCIAL_INSURANCE into words. Values
// with any lowercase letter are left exactly as the source wrote them.
func readable(s string) string {
	if strings.ToUpper(s) != s || !strings.Contains(s, "_") {
		return s
	}
	words := strings.Fields(strings.ReplaceAll(s, "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}
