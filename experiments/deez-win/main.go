// deez.win — a multiplayer quiz where the questions come from a verified
// entity graph rather than from a language model.
//
// The loop: players join a room, dice decide the order, the winner picks an
// overall topic, and everyone else claims one of the top five sub-topics Cala
// actually holds data for. Questions are built from those axes.
package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed static
var staticFS embed.FS

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	d3bitURL := flag.String("d3bit", envOr("D3BIT_URL", "https://api.d3bit.com"), "D3BIT API base URL")
	origin := flag.String("origin", "", "public origin for auth callbacks (default http://localhost<addr>)")
	statePath := flag.String("leaderboard", defaultLeaderboardPath(), "path to the persisted leaderboard")
	devAuth := flag.Bool("dev-auth", false, "use local stand-in sessions instead of D3BIT (local development only)")
	flag.Parse()

	cala := NewCala(os.Getenv("CALA_API_KEY"))
	fal := NewFAL(os.Getenv("FAL_KEY"))

	var auth sessionSource = NewAuth(*d3bitURL)
	if *devAuth {
		auth = NewDevAuth()
		log.Printf("auth: DEV MODE — anyone can mint a session. Never expose this publicly.")
	} else {
		log.Printf("auth: d3bit %s", *d3bitURL)
	}

	if *origin == "" {
		*origin = "http://localhost" + *addr
	}

	board := NewLeaderboard(*statePath)
	log.Printf("leaderboard: %s", *statePath)

	srv := &Server{
		auth:     auth,
		board:    board,
		d3bitURL: *d3bitURL,
		origin:   *origin,
		store:    NewStore(),
		cala:     cala,
		fal:      fal,
	}

	mux := http.NewServeMux()
	srv.Routes(mux)

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("static assets: %v", err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(sub)))

	if cala.Enabled() {
		log.Printf("cala: enabled")
	} else {
		log.Printf("cala: no CALA_API_KEY — running on offline fixtures")
	}
	if fal.Enabled() {
		log.Printf("fal: enabled for the first 2 questions")
	} else {
		log.Printf("fal: no FAL_KEY — questions remain text-only")
	}
	log.Printf("listening on http://localhost%s", *addr)

	server := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
