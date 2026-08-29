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
	if !strings.Contains(html, `/dice-d6.png`) || !strings.Contains(html, "6 SIDES") || strings.Contains(html, "d12") {
		t.Fatal("home does not use the six-sided die identity")
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
