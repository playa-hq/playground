package main

import (
	"bytes"
	"image"
	_ "image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersIncludeHSTS(t *testing.T) {
	handler := withSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://deez.win/", nil))

	if got := recorder.Header().Get("Strict-Transport-Security"); got != "max-age=31536000; includeSubDomains" {
		t.Fatalf("Strict-Transport-Security = %q", got)
	}
}

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

func TestAuraAndLarp(t *testing.T) {
	tests := []struct {
		name    string
		correct bool
		elapsed int
		seeded  bool
		want    int
	}{
		{"fast correct earns max aura", true, 0, false, 150},
		{"slow correct earns base aura", true, 12000, false, 100},
		{"seeded axis earns half aura", true, 0, true, 75},
		{"miss applies larp", false, 0, false, -25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := auraDelta(tt.correct, tt.elapsed, tt.seeded); got != tt.want {
				t.Fatalf("auraDelta() = %d, want %d", got, tt.want)
			}
		})
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
	if !strings.Contains(html, `/dice-d6.png`) || strings.Contains(html, "6 SIDES") || strings.Contains(html, "d12") {
		t.Fatal("home does not use the six-sided die identity")
	}
	for _, removed := range []string{"A data-backed party game", "Roll for the order", "↗"} {
		if strings.Contains(html, removed) {
			t.Fatalf("home still contains removed copy %q", removed)
		}
	}
	for _, want := range []string{"Trivia for friends", "Pick a topic. Outsmart your friends.", "DATA BY CALA", "IMAGES &amp; SOUND BY FAL.AI"} {
		if !strings.Contains(html, want) {
			t.Fatalf("home is missing %q", want)
		}
	}
	if strings.Count(html, `/home.js`) != 1 {
		t.Fatal("shared button motion must load exactly once")
	}
}

func TestSixSidedDiceAssets(t *testing.T) {
	for path, wantSize := range map[string]int{
		"static/favicon.png": 128,
		"static/dice-d6.png": 720,
	} {
		f, err := staticFS.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		config, _, err := image.DecodeConfig(f)
		f.Close()
		if err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if config.Width != wantSize || config.Height != wantSize {
			t.Errorf("%s is %dx%d, want %dx%d", path, config.Width, config.Height, wantSize, wantSize)
		}
	}
}

func TestSharedDarkThemeIncludesMobileLayout(t *testing.T) {
	css, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	styles := string(css)
	for _, want := range []string{
		"--bg: #07080A",
		"--panel: #101216",
		"--accent: #72F1C6",
		"animation: hero-die-roll",
		"@keyframes hero-die-roll",
		"@media (max-width: 560px)",
		"min-height: 44px",
	} {
		if !strings.Contains(styles, want) {
			t.Errorf("shared theme is missing %q", want)
		}
	}
}

func TestSharedButtonWiggleUsesDelegation(t *testing.T) {
	js, err := staticFS.ReadFile("static/home.js")
	if err != nil {
		t.Fatal(err)
	}
	motion := string(js)
	for _, want := range []string{"button, .btn", "pointermove", "pointerout"} {
		if !strings.Contains(motion, want) {
			t.Errorf("shared button motion is missing %q", want)
		}
	}
}

func TestNegativeAuraStillProducesAWinner(t *testing.T) {
	board := NewLeaderboard("")
	board.Record(&Room{Players: []*Player{
		{ID: "least-larp", DisplayName: "Least Larp", Score: -25},
		{ID: "most-larp", DisplayName: "Most Larp", Score: -50},
	}})
	top := board.Top(2)
	if len(top) != 2 || top[0].PlayerID != "least-larp" || top[0].Wins != 1 {
		t.Fatalf("negative-aura standings did not preserve the winner: %+v", top)
	}
}

func TestAnswerReceiptNamesAuraAndLarp(t *testing.T) {
	for name, question := range map[string]*questionView{
		"aura": {Answered: true, WasCorrect: true, Aura: 140},
		"larp": {Answered: true, WasCorrect: false, Larp: 25},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			v := &roomView{Room: &Room{Phase: PhaseQuiz, QuestionCount: 1}, Question: question}
			if err := panelTmpl.ExecuteTemplate(&out, "panel", v); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"aura": "+140 AURA", "larp": "25 LARP"}[name]
			if !strings.Contains(out.String(), want) {
				t.Fatalf("answer receipt is missing %q", want)
			}
		})
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

func TestResultsRenderReceiptRoll(t *testing.T) {
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
		`class="receipt-roll"`,
		`class="roll-spool"`,
		`class="receipt-paper"`,
		`class="receipt-rolled-edge"`,
		"RECEIPT · Cities",
		"Lisbon was founded around 1200 BC.",
		"every line above has a source",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered results are missing %q", want)
		}
	}
	if strings.Contains(html, "printer-slot") {
		t.Error("receipt roll still renders the old printer slot")
	}
}

func TestRoomLeaveReturnsToLobbyPage(t *testing.T) {
	var out bytes.Buffer
	v := &roomView{Room: &Room{Code: "ABCD", Phase: PhaseLobby, MaxPlayer: 2}, FullPage: true}
	if err := roomTmpl.ExecuteTemplate(&out, "layout", v); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	if !strings.Contains(html, `<a class="btn ghost small" href="/play">Leave</a>`) {
		t.Fatal("room Leave action does not return to the lobby page")
	}
}
