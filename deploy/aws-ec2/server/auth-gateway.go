package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const sessionCookie = "video_studio_session"

type gateway struct {
	secret      []byte
	password    []byte
	frontendDir string
	agent       *httputil.ReverseProxy
	workspace   *httputil.ReverseProxy
}

// gatewayClaims deliberately mirrors only the identity fields consumed by the
// agent API. The password session remains the public-facing authentication
// boundary; this signed, short-lived token is created server-side solely for
// the loopback hop from the gateway to the agent API.
type gatewayClaims struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Provider  string `json:"provider"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

func proxyFor(rawURL string) *httputil.ReverseProxy {
	target, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return httputil.NewSingleHostReverseProxy(target)
}

func newGateway() *gateway {
	secret := os.Getenv("AUTH_SECRET")
	password := os.Getenv("ACCESS_PASSWORD")
	if len(secret) < 32 || password == "" {
		log.Fatal("AUTH_SECRET (32+ chars) and ACCESS_PASSWORD are required")
	}
	return &gateway{
		secret:      []byte(secret),
		password:    []byte(password),
		frontendDir: os.Getenv("FRONTEND_DIR"),
		agent:       proxyFor("http://127.0.0.1:8000"),
		workspace:   proxyFor("http://127.0.0.1:8080"),
	}
}

func (g *gateway) serveFrontend(w http.ResponseWriter, r *http.Request) {
	cleanPath := filepath.Clean("/" + r.URL.Path)
	path := filepath.Join(g.frontendDir, cleanPath)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		// Release deployments swap the frontend directory behind a stable URL.
		// The HTML entry point selects hashed JS assets, so it must never be
		// served from a previous browser cache after a release.
		if filepath.Base(path) == "index.html" || filepath.Base(path) == "runtime-config.js" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFile(w, r, path)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join(g.frontendDir, "index.html"))
}

func (g *gateway) signedSession(expires time.Time) string {
	payload := strconv.FormatInt(expires.Unix(), 10)
	mac := hmac.New(sha256.New, g.secret)
	_, _ = mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (g *gateway) validSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return false
	}
	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().After(time.Unix(expires, 0)) {
		return false
	}
	want := g.signedSession(time.Unix(expires, 0))
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(want)) == 1
}

func (g *gateway) agentToken() (string, error) {
	now := time.Now()
	claims := gatewayClaims{
		UserID:    "video-studio",
		Username:  "video-studio",
		Provider:  "gateway",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(15 * time.Minute).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := header + "." + body
	mac := hmac.New(sha256.New, g.secret)
	_, _ = mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (g *gateway) serveAgent(w http.ResponseWriter, r *http.Request) {
	// An explicit bearer token wins. Public file links and browser SSE cannot
	// attach a custom header, so the agent API also supports ?token=. Preserve
	// that token as a bearer credential instead of replacing it with the
	// gateway's service identity; the token determines which user's workspace
	// the public file handler reads from.
	//
	// Browser users without either token authenticate through the HttpOnly
	// password-session cookie, so the gateway supplies the signed internal
	// token required by the agent API on their behalf.
	if r.Header.Get("Authorization") == "" {
		if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		} else {
			token, err := g.agentToken()
			if err != nil {
				http.Error(w, "Unable to authenticate gateway request", http.StatusInternalServerError)
				return
			}
			r.Header.Set("Authorization", "Bearer "+token)
		}
	}
	g.agent.ServeHTTP(w, r)
}

func safeNext(next string) string {
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") {
		return next
	}
	return "/"
}

func (g *gateway) login(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r.URL.Query().Get("next"))
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err == nil && subtle.ConstantTimeCompare([]byte(r.Form.Get("password")), g.password) == 1 {
			expires := time.Now().Add(12 * time.Hour)
			http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: g.signedSession(expires), Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, Expires: expires})
			http.Redirect(w, r, safeNext(r.Form.Get("next")), http.StatusSeeOther)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "<!doctype html><html><head><meta name=viewport content='width=device-width,initial-scale=1'><title>Video Studio</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#101827;color:#eef2ff;font:16px system-ui}main{width:min(360px,calc(100% - 48px));padding:32px;border:1px solid #334155;border-radius:16px;background:#172033}input,button{width:100%;box-sizing:border-box;padding:12px;border-radius:8px;font:inherit}input{border:1px solid #475569;background:#0f172a;color:white;margin:16px 0}button{border:0;background:#38bdf8;color:#082f49;font-weight:700;cursor:pointer}.error{color:#fda4af}</style></head><body><main><h1>Video Studio</h1><p>Enter the shared access password.</p>")
	if r.Method == http.MethodPost {
		fmt.Fprint(w, "<p class=error>Incorrect password. Try again.</p>")
	}
	fmt.Fprintf(w, "<form method=post><input type=hidden name=next value=%q><input type=password name=password autofocus required><button type=submit>Continue</button></form></main></body></html>", next)
}

func (g *gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/login" {
		g.login(w, r)
		return
	}
	if r.URL.Path == "/logout" {
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !g.validSession(r) {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/wp"):
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api/wp")
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
		g.workspace.ServeHTTP(w, r)
	case strings.HasPrefix(r.URL.Path, "/api"), strings.HasPrefix(r.URL.Path, "/ws"):
		g.serveAgent(w, r)
	default:
		g.serveFrontend(w, r)
	}
}

func main() {
	server := &http.Server{Addr: ":8090", Handler: newGateway(), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(server.ListenAndServe())
}
