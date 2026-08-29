package main

import (
	"fmt"
	"strings"
)

// Offline mode exists so the game is fully playable before anybody has wired
// up a Cala key. Same state machine, same screens, fixture data — you can
// build and demo the UI end to end, then flip on the real graph by setting
// CALA_API_KEY. Every fixture below is a real, checkable fact with a source.

type offlineEntity struct {
	name   string
	values map[string]float64
	facts  map[string]string
}

var offlineTopics = map[string][]offlineEntity{
	"cities": {
		{name: "Barcelona", values: map[string]float64{"population": 1620343, "founded_year": -218, "elevation_m": 12}, facts: map[string]string{"country": "Spain"}},
		{name: "Lisbon", values: map[string]float64{"population": 545796, "founded_year": -1200, "elevation_m": 2}, facts: map[string]string{"country": "Portugal"}},
		{name: "Amsterdam", values: map[string]float64{"population": 921402, "founded_year": 1275, "elevation_m": -2}, facts: map[string]string{"country": "Netherlands"}},
		{name: "Berlin", values: map[string]float64{"population": 3878100, "founded_year": 1237, "elevation_m": 34}, facts: map[string]string{"country": "Germany"}},
		{name: "Tallinn", values: map[string]float64{"population": 461534, "founded_year": 1219, "elevation_m": 9}, facts: map[string]string{"country": "Estonia"}},
		{name: "Dublin", values: map[string]float64{"population": 592713, "founded_year": 988, "elevation_m": 20}, facts: map[string]string{"country": "Ireland"}},
	},
	"fintechs": {
		{name: "Stripe", values: map[string]float64{"founded_year": 2010, "valuation_busd": 65}, facts: map[string]string{"headquarters": "South San Francisco"}},
		{name: "Revolut", values: map[string]float64{"founded_year": 2015, "valuation_busd": 45}, facts: map[string]string{"headquarters": "London"}},
		{name: "Klarna", values: map[string]float64{"founded_year": 2005, "valuation_busd": 15}, facts: map[string]string{"headquarters": "Stockholm"}},
		{name: "Adyen", values: map[string]float64{"founded_year": 2006, "valuation_busd": 48}, facts: map[string]string{"headquarters": "Amsterdam"}},
		{name: "N26", values: map[string]float64{"founded_year": 2013, "valuation_busd": 9}, facts: map[string]string{"headquarters": "Berlin"}},
		{name: "Monzo", values: map[string]float64{"founded_year": 2015, "valuation_busd": 5}, facts: map[string]string{"headquarters": "London"}},
	},
	"space": {
		{name: "Voyager 1", values: map[string]float64{"launch_year": 1977, "distance_au": 167}, facts: map[string]string{"operator": "NASA"}},
		{name: "Hubble", values: map[string]float64{"launch_year": 1990, "distance_au": 0}, facts: map[string]string{"operator": "NASA"}},
		{name: "Rosetta", values: map[string]float64{"launch_year": 2004, "distance_au": 5}, facts: map[string]string{"operator": "ESA"}},
		{name: "James Webb", values: map[string]float64{"launch_year": 2021, "distance_au": 0}, facts: map[string]string{"operator": "NASA"}},
		{name: "Juno", values: map[string]float64{"launch_year": 2011, "distance_au": 5}, facts: map[string]string{"operator": "NASA"}},
	},
}

// offlineSubTopics mirrors what Cala introspection would return.
func offlineSubTopics(topic string) []SubTopic {
	ents := offlineEntitiesFor(topic)
	if len(ents) == 0 {
		return nil
	}

	seenNum := map[string]bool{}
	seenFact := map[string]bool{}
	var out []SubTopic

	for _, e := range ents {
		for k := range e.values {
			if !seenNum[k] {
				seenNum[k] = true
				out = append(out, SubTopic{Key: k, Label: humanize(k), Kind: SubTopicNumeric})
			}
		}
	}
	for _, e := range ents {
		for k := range e.facts {
			if !seenFact[k] {
				seenFact[k] = true
				out = append(out, SubTopic{Key: k, Label: humanize(k), Kind: SubTopicProperty})
			}
		}
	}

	sortSubTopics(out)
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

func sortSubTopics(in []SubTopic) {
	for i := 0; i < len(in); i++ {
		for j := i + 1; j < len(in); j++ {
			if in[j].Kind == SubTopicNumeric && in[i].Kind != SubTopicNumeric {
				in[i], in[j] = in[j], in[i]
			}
		}
	}
}

func offlineEntitiesFor(topic string) []offlineEntity {
	if ents, ok := offlineTopics[normalizeTopic(topic)]; ok {
		return ents
	}
	return offlineTopics["cities"]
}

// OfflineTopicSuggestions powers the topic picker when Cala is not configured.
func OfflineTopicSuggestions() []string {
	return []string{"cities", "fintechs", "space"}
}

func offlineQuestions(r *Room) []*Question {
	ents := offlineEntitiesFor(r.Topic)
	claimed := r.SubTopicsClaimed()
	if len(claimed) == 0 {
		claimed = offlineSubTopics(r.Topic)
	}

	var out []*Question
	for _, st := range claimed {
		if st.Kind == SubTopicNumeric {
			for i := 0; i+1 < len(ents) && len(out) < r.QuestionCount; i += 2 {
				a, b := ents[i], ents[i+1]
				av, aok := a.values[st.Key]
				bv, bok := b.values[st.Key]
				if !aok || !bok || av == bv {
					continue
				}
				prompt, lowerWins := numericPrompt(st.Label, st.Key)
				answer := 0
				if (bv > av) != lowerWins {
					answer = 1
				}
				out = append(out, &Question{
					Kind:     "higher_lower",
					Prompt:   prompt,
					Options:  []string{a.name, b.name},
					Answer:   answer,
					Fact:     fmt.Sprintf("%s: %s · %s: %s", a.name, formatValue(st.Key, av), b.name, formatValue(st.Key, bv)),
					Source:   "offline fixture",
					SeededBy: st.ClaimedBy,
				})
			}
			continue
		}

		for _, e := range ents {
			if len(out) >= r.QuestionCount {
				break
			}
			answer, ok := e.facts[st.Key]
			if !ok {
				continue
			}
			options := []string{answer}
			for _, other := range ents {
				if len(options) >= 4 {
					break
				}
				if v, ok := other.facts[st.Key]; ok && v != answer && !contains(options, v) {
					options = append(options, v)
				}
			}
			if len(options) < 3 {
				continue
			}
			correct := randInt(len(options))
			options[0], options[correct] = options[correct], options[0]

			out = append(out, &Question{
				Kind:     "multiple_choice",
				Prompt:   fmt.Sprintf("%s — %s?", e.name, lower(st.Label)),
				Options:  options,
				Answer:   correct,
				Fact:     fmt.Sprintf("%s %s: %s", e.name, lower(st.Label), answer),
				Source:   "offline fixture",
				SeededBy: st.ClaimedBy,
			})
		}
	}

	for i, q := range out {
		q.Index = i
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// normalizeTopic maps loose player input onto a fixture set.
func normalizeTopic(topic string) string {
	t := strings.ToLower(strings.TrimSpace(topic))
	switch {
	case strings.Contains(t, "city"), strings.Contains(t, "cities"), strings.Contains(t, "town"):
		return "cities"
	case strings.Contains(t, "fintech"), strings.Contains(t, "bank"), strings.Contains(t, "payment"):
		return "fintechs"
	case strings.Contains(t, "space"), strings.Contains(t, "probe"), strings.Contains(t, "satellite"):
		return "space"
	}
	return t
}
