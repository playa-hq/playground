package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockCala serves the response shapes documented at docs.cala.ai for three
// companies, so the whole topic → axes → questions pipeline runs offline.
func mockCala(t *testing.T) *Cala {
	t.Helper()
	type co struct {
		id, name, hq, country string
		employees, founded    float64
		revenue               float64
	}
	cos := []co{
		{"a1", "APPLE INC", "One Apple Park Way, Cupertino", "United States", 166000, 1977, 391e9},
		{"b2", "MICROSOFT CORP", "One Microsoft Way, Redmond", "United States", 228000, 1975, 245e9},
		{"c3", "SAP SE", "Dietmar-Hopp-Allee 16, Walldorf", "Germany", 109000, 1972, 34e9},
	}
	byID := map[string]co{}
	for _, c := range cos {
		byID[c.id] = c
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/knowledge/query", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "test" {
			http.Error(w, "no key", 401)
			return
		}
		var ents []map[string]any
		for _, c := range cos {
			ents = append(ents, map[string]any{"id": c.id, "name": c.name, "entity_type": "Company"})
		}
		json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "entities": ents})
	})
	mux.HandleFunc("GET /v1/entities/{id}/introspection", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"properties":    []string{"cik", "headquarters_address", "founding_date", "legal_name", "employee_count", "name", "id"},
			"relationships": map[string]any{"outgoing": []string{"IS_REGISTERED_IN", "IS_ULTIMATE_PARENT_OF"}, "incoming": []string{"IS_CEO_OF"}},
			"numerical_observations": map[string]any{"FinancialMetric": []map[string]any{
				{"id": "rev-" + r.PathValue("id"), "name": "Revenues", "unit": "USD", "cadence": "a"},
			}},
		})
	})
	mux.HandleFunc("POST /v1/entities/{id}", func(w http.ResponseWriter, r *http.Request) {
		c := byID[r.PathValue("id")]
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["numerical_observations"].(map[string]any)["FinancialMetric"]; !ok {
			t.Errorf("metric ids not requested: %v", body)
		}
		src := []map[string]any{{"date": "2026-04-15", "document": "https://www.sec.gov/x", "name": "SEC"}}
		json.NewEncoder(w).Encode(map[string]any{
			"id": c.id, "name": c.name,
			"properties": map[string]any{
				"headquarters_address": map[string]any{"value": c.hq, "sources": src},
				"founding_date":        map[string]any{"value": fmt.Sprintf("%d-01-01", int(c.founded)), "sources": src},
				"employee_count":       map[string]any{"value": c.employees, "sources": src},
			},
			"relationships": map[string]any{"outgoing": map[string]any{
				"IS_REGISTERED_IN": []map[string]any{{"name": c.country, "entity_type": "Country", "properties": map[string]any{"sources": src}}},
			}},
			"numerical_observations": []map[string]any{{
				"name": "Revenues", "type": "FinancialMetric", "properties": map[string]any{"unit": "USD"},
				"data": []map[string]any{
					{"time": "2024-09-28T00:00:00Z", "value": c.revenue * 0.9, "origin": map[string]any{"name": "10-K", "source": "SEC", "url": "https://sec.gov/old"}},
					{"time": "2025-09-27T00:00:00Z", "value": c.revenue, "origin": map[string]any{"name": "10-K", "source": "SEC", "url": "https://sec.gov/new"}},
				},
			}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &Cala{apiKey: "test", baseURL: srv.URL, client: srv.Client()}
}

func TestPipelineAgainstDocumentedShapes(t *testing.T) {
	c := mockCala(t)
	g, err := c.BuildTopicGraph(context.Background(), "Big Tech", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Entities) != 3 || len(g.Intro) != 3 {
		t.Fatalf("graph: %d entities, %d introspections", len(g.Entities), len(g.Intro))
	}

	subs := g.SubTopics(5)
	got := map[string]SubTopicKind{}
	for _, st := range subs {
		got[st.Key] = st.Kind
	}
	for key, kind := range map[string]SubTopicKind{
		"employee_count": SubTopicNumeric, "founding_date": SubTopicNumeric,
		"Revenues": SubTopicMetric, "IS_REGISTERED_IN": SubTopicRelation, "headquarters_address": SubTopicProperty,
	} {
		if got[key] != kind {
			t.Errorf("axis %s: got %q want %q (all: %v)", key, got[key], kind, subs)
		}
	}
	if _, ok := got["cik"]; ok {
		t.Error("boring property offered")
	}
	if _, ok := got["IS_ULTIMATE_PARENT_OF"]; ok {
		t.Error("boring relation offered")
	}

	for i := range subs {
		subs[i].ClaimedBy = "p"
	}
	g.Fetch(context.Background(), c, subs, nil)
	details := g.Details()
	if len(details) != 3 {
		t.Fatalf("details: %d", len(details))
	}

	var all []*Question
	for _, st := range subs {
		qs := axisQuestions(st, details, 2)
		if len(qs) == 0 {
			t.Errorf("axis %s produced no questions", st.Key)
		}
		all = append(all, qs...)
	}
	for _, q := range all {
		if q.Source == "" || q.SourceURL == "" {
			t.Errorf("question without a source: %+v", q)
		}
		if q.Answer < 0 || q.Answer >= len(q.Options) {
			t.Errorf("answer out of range: %+v", q)
		}
		switch {
		case q.Prompt == "Which was founded first?":
			if !strings.Contains(q.Fact, "1972") && !strings.Contains(q.Fact, "1975") {
				t.Errorf("founded fact lost the year: %s", q.Fact)
			}
		case strings.Contains(q.Prompt, "revenues"):
			if !strings.Contains(q.Fact, "B ·") || q.SourceURL != "https://sec.gov/new" {
				t.Errorf("revenue should use latest point with unit: %s [%s]", q.Fact, q.SourceURL)
			}
		case strings.Contains(q.Prompt, "registered"):
			if !contains(q.Options, "Germany") || !contains(q.Options, "United States") {
				t.Errorf("relation distractors should be other entities' values: %v", q.Options)
			}
		}
	}
}

func TestReadable(t *testing.T) {
	for in, want := range map[string]string{"FINANCIAL_INSURANCE": "Financial Insurance", "Madrid": "Madrid", "SAP SE": "SAP SE", "FINTECH": "FINTECH"} {
		if got := readable(in); got != want {
			t.Errorf("readable(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatUnit(t *testing.T) {
	cases := map[string]string{
		formatUnit("revenue", 391035000000, "USD"): "$391.0B",
		formatUnit("employee_count", 166000, ""):   "166,000",
		formatUnit("founding_date", -218, ""):      "218 BC",
		formatUnit("x", 1234.5, "EUR"):             "€1,234.50",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
}
