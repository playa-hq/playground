package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// D3bitUser mirrors the profile returned by GET /auth/me.
type D3bitUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	AvatarURL   string `json:"avatar_url"`
	IsAnon      bool   `json:"is_anon"`
	DisplayName string `json:"display_name"`
	Color       string `json:"color"`
	Username    string `json:"username"`
}

// isSecureRequest returns true if the request arrived over HTTPS.
// Nginx sets X-Forwarded-Proto when proxying; localhost defaults to false.
func isSecureRequest(r *http.Request) bool {
	return r.Header.Get("X-Forwarded-Proto") == "https"
}

type cachedUser struct {
	user      *D3bitUser
	expiresAt time.Time
}

// Auth wraps the D3BIT API: a same-origin proxy for the browser plus a
// server-side session resolver with a short-lived cache.
type Auth struct {
	baseURL string

	mu    sync.RWMutex
	cache map[string]*cachedUser
}

func NewAuth(baseURL string) *Auth {
	return &Auth{baseURL: baseURL, cache: make(map[string]*cachedUser)}
}

// Routes registers the browser-facing auth surface.
//
// Everything goes through this origin rather than hitting D3BIT directly:
// D3BIT sends Access-Control-Allow-Origin: https://d3bit.com, so a browser
// fetch from anywhere else is blocked. Server-to-server has no such problem,
// and proxying also lets us re-scope the session cookie to our own host.
func (a *Auth) Routes(mux *http.ServeMux) {
	for _, r := range []struct{ pattern string }{
		{"GET /d3bit/auth/me"},
		{"PATCH /d3bit/auth/me"},
		{"POST /d3bit/auth/anon"},
		{"POST /d3bit/auth/login"},
		{"POST /d3bit/auth/claim"},
		{"POST /d3bit/auth/logout"},
	} {
		mux.HandleFunc(r.pattern, a.proxy)
	}
	mux.HandleFunc("GET /auth/callback", a.callback)
}

func (a *Auth) proxy(w http.ResponseWriter, r *http.Request) {
	// A profile edit invalidates whatever we cached for this session.
	if r.Method == http.MethodPatch {
		if c, err := r.Cookie("d3_session"); err == nil {
			a.mu.Lock()
			delete(a.cache, c.Value)
			a.mu.Unlock()
		}
	}

	target := a.baseURL + strings.TrimPrefix(r.URL.Path, "/d3bit")
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		httpError(w, http.StatusBadGateway, "proxy_error", "Could not build the upstream request.")
		return
	}
	req.Header = r.Header.Clone()
	for _, c := range r.Cookies() {
		req.AddCookie(c)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		httpError(w, http.StatusBadGateway, "d3bit_unreachable", "D3BIT is not reachable.")
		return
	}
	defer resp.Body.Close()

	secure := isSecureRequest(r)
	for k, vv := range resp.Header {
		for _, v := range vv {
			if strings.EqualFold(k, "Set-Cookie") {
				w.Header().Add("Set-Cookie", rewriteCookie(v, secure))
				continue
			}
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// callback lands the Google OAuth redirect: D3BIT sends the browser back here
// with ?code=, we trade it for a session token server-side and set the cookie
// on our own origin.
func (a *Auth) callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		httpError(w, http.StatusBadRequest, "missing_code", "No authorization code in the callback.")
		return
	}

	req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, a.baseURL+"/auth/exchange?code="+code, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		httpError(w, http.StatusBadGateway, "exchange_failed", "Could not exchange the code for a session.")
		return
	}
	defer resp.Body.Close()

	var out struct {
		Data struct {
			SessionToken string `json:"session_token"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if out.Data.SessionToken == "" {
		httpError(w, http.StatusBadGateway, "no_session", "The exchange returned no session token.")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "d3_session",
		Value:    out.Data.SessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		MaxAge:   30 * 24 * 60 * 60,
		SameSite: http.SameSiteLaxMode,
	})

	next := r.URL.Query().Get("next")
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// Bootstrap mints an anonymous D3BIT account and plants the session cookie on
// this origin. Called server-side on first page view so a player never sees a
// login wall — they get a name and a colour and start playing.
func (a *Auth) Bootstrap(w http.ResponseWriter, r *http.Request) *D3bitUser {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, a.baseURL+"/auth/anon", nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()

	var env struct {
		Data struct {
			User D3bitUser `json:"user"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&env) != nil || env.Data.User.ID == "" {
		return nil
	}
	secure := isSecureRequest(r)
	for _, c := range resp.Cookies() {
		if c.Name == "d3_session" {
			http.SetCookie(w, &http.Cookie{
				Name:     c.Name,
				Value:    c.Value,
				Path:     "/",
				HttpOnly: true,
				Secure:   secure,
				MaxAge:   30 * 24 * 60 * 60,
				SameSite: http.SameSiteLaxMode,
			})
			// Cache under the new token so the very next call resolves.
			a.mu.Lock()
			a.cache[c.Value] = &cachedUser{user: &env.Data.User, expiresAt: time.Now().Add(60 * time.Second)}
			a.mu.Unlock()
		}
	}
	return &env.Data.User
}

// SendMagicLink asks D3BIT to email a sign-in link back to our callback.
func (a *Auth) SendMagicLink(ctx context.Context, email, redirect string) error {
	body, _ := json.Marshal(map[string]string{"email": email, "redirect": redirect})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/auth/login", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.New("D3BIT is not reachable right now.")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("Could not send the sign-in link.")
	}
	return nil
}

// User resolves the caller's D3BIT profile, or nil when unauthenticated.
func (a *Auth) User(r *http.Request) *D3bitUser {
	c, err := r.Cookie("d3_session")
	if err != nil || c.Value == "" {
		return nil
	}

	a.mu.RLock()
	if hit, ok := a.cache[c.Value]; ok && time.Now().Before(hit.expiresAt) {
		a.mu.RUnlock()
		return hit.user
	}
	a.mu.RUnlock()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, a.baseURL+"/auth/me", nil)
	if err != nil {
		return nil
	}
	req.AddCookie(c)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()

	var env struct {
		Data D3bitUser `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&env) != nil || env.Data.ID == "" {
		return nil
	}

	a.mu.Lock()
	a.cache[c.Value] = &cachedUser{user: &env.Data, expiresAt: time.Now().Add(60 * time.Second)}
	a.mu.Unlock()
	return &env.Data
}

// rewriteCookie re-scopes an upstream Set-Cookie to this origin: Domain is
// dropped, and SameSite=None (which requires Secure) becomes Lax. When the
// request arrived over HTTPS, Secure is preserved; otherwise it is stripped
// to allow local development over plain HTTP.
func rewriteCookie(setCookie string, secure bool) string {
	var out []string
	for _, part := range strings.Split(setCookie, ";") {
		trimmed := strings.TrimSpace(part)
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "domain="):
			continue
		case lower == "secure":
			if secure {
				out = append(out, trimmed)
			}
			continue
		case strings.HasPrefix(lower, "samesite=none"):
			out = append(out, "SameSite=Lax")
		default:
			out = append(out, trimmed)
		}
	}
	return strings.Join(out, "; ")
}

// Logout clears the session upstream and on this origin.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("d3_session"); err == nil {
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, a.baseURL+"/auth/logout", nil)
		req.AddCookie(c)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
		a.mu.Lock()
		delete(a.cache, c.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "d3_session", Value: "", Path: "/", MaxAge: -1})
}
