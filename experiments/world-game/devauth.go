package main

import (
	"net/http"
	"sync"
)

// Dev auth is a local stand-in for D3BIT, enabled only by -dev-auth.
//
// It exists because D3BIT's /auth/anon is not always reachable (it is 500ing
// on production as of this writing) and a hackathon team should not be blocked
// on someone else's outage. It is off by default and must never be enabled on
// anything public: it hands out a session to whoever asks, with no verification.
type DevAuth struct {
	mu       sync.Mutex
	sessions map[string]*D3bitUser
	seq      int
}

var devNames = []string{"Pixel", "Comet", "Marble", "Falcon", "Tundra", "Nimbus", "Cobalt", "Sorrel"}
var devColors = []string{"#7cf5c8", "#ffd166", "#ff6b8a", "#7c9cff", "#c792ea", "#f78c6c"}

func NewDevAuth() *DevAuth {
	return &DevAuth{sessions: make(map[string]*D3bitUser)}
}

func (d *DevAuth) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /d3bit/auth/me", d.me)
	mux.HandleFunc("POST /d3bit/auth/anon", d.anon)
	mux.HandleFunc("POST /d3bit/auth/logout", d.logout)

	// Login paths exist so the UI behaves identically; they are inert here.
	notSupported := func(w http.ResponseWriter, r *http.Request) {
		httpError(w, http.StatusNotImplemented, "dev_auth",
			"Dev auth has no email or Google sign-in. Point -d3bit at a real D3BIT instance.")
	}
	mux.HandleFunc("POST /d3bit/auth/login", notSupported)
	mux.HandleFunc("POST /d3bit/auth/claim", notSupported)
	mux.HandleFunc("PATCH /d3bit/auth/me", notSupported)
}

func (d *DevAuth) anon(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	d.seq++
	u := &D3bitUser{
		ID:          "dev_" + newCode() + newCode(),
		IsAnon:      true,
		DisplayName: devNames[d.seq%len(devNames)],
		Color:       devColors[d.seq%len(devColors)],
	}
	token := "devsess_" + newCode() + newCode() + newCode()
	d.sessions[token] = u
	d.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name: "d3_session", Value: token, Path: "/",
		HttpOnly: true, MaxAge: 86400, SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"user": u}})
}

func (d *DevAuth) me(w http.ResponseWriter, r *http.Request) {
	u := d.User(r)
	if u == nil {
		httpError(w, http.StatusUnauthorized, "unauthenticated", "No session.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": u})
}

func (d *DevAuth) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("d3_session"); err == nil {
		d.mu.Lock()
		delete(d.sessions, c.Value)
		d.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "d3_session", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (d *DevAuth) User(r *http.Request) *D3bitUser {
	c, err := r.Cookie("d3_session")
	if err != nil || c.Value == "" {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sessions[c.Value]
}
