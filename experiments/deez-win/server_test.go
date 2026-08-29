package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestQuestionCount(t *testing.T) {
	tests := map[string]int{
		"5":   5,
		"10":  10,
		"15":  15,
		"":    10,
		"8":   10,
		"100": 10,
	}
	for raw, want := range tests {
		if got := questionCount(raw); got != want {
			t.Errorf("questionCount(%q) = %d, want %d", raw, got, want)
		}
	}
}

func TestHomeHasOnlyPlayAction(t *testing.T) {
	var out bytes.Buffer
	v := homeView{Letters: strings.Split("DEEZ.WIN", "")}
	if err := homeTmpl.ExecuteTemplate(&out, "layout", v); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	if got := strings.Count(html, "<button"); got != 1 {
		t.Fatalf("home has %d buttons, want only Play", got)
	}
	if !strings.Contains(html, `action="/play"`) || !strings.Contains(html, ">Play</span>") {
		t.Fatal("home Play action does not lead to the lobby page")
	}
}

func TestLobbyOffersQuestionCounts(t *testing.T) {
	var out bytes.Buffer
	v := homeView{PlayerCounts: []int{2, 3, 4}, QuestionCounts: []int{5, 10, 15}}
	if err := lobbyTmpl.ExecuteTemplate(&out, "layout", v); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, count := range []string{"5", "10", "15"} {
		want := `name="question_count" value="` + count + `"`
		if !strings.Contains(html, want) {
			t.Errorf("lobby is missing %s-question option", count)
		}
	}
}

func TestResultsRenderReceiptPrinter(t *testing.T) {
	var out bytes.Buffer
	v := &roomView{
		Room: &Room{Phase: PhaseResults, Topic: "Cities"},
		Review: []reviewView{{
			Prompt: "Which city is older?",
			Answer: "Lisbon",
			Fact:   "Lisbon was founded around 1200 BC.",
			Source: "offline fixture",
		}},
	}
	if err := panelTmpl.ExecuteTemplate(&out, "panel", v); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{
		`class="receipt-printer"`,
		`class="printer-slot"`,
		`class="receipt-paper"`,
		"RECEIPT · Cities",
		"Lisbon was founded around 1200 BC.",
		"every line above has a source",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered results are missing %q", want)
		}
	}
}
