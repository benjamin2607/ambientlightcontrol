// Package api serves the JSON API used by the macOS app. Every endpoint except /healthz requires
// the bearer token.
package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/benjamin2607/ambientlightcontrol/server/internal/controller"
	"github.com/benjamin2607/ambientlightcontrol/server/internal/store"
)

// Controller is what the handlers need from the controller package.
type Controller interface {
	Status() controller.Status
	SetLight(ctx context.Context, r controller.LightRequest) error
	UpdateSettings(p controller.SettingsPatch) (store.Settings, error)
	ResumeAutomation()
	PairTV(ctx context.Context, host string) error
}

const (
	maxBodyBytes   = 16 << 10
	pairingTimeout = 60 * time.Second
)

type Server struct {
	ctrl      Controller
	tokenHash [32]byte
	log       *slog.Logger
	version   string
	failures  failureLimiter
}

func New(ctrl Controller, token string, version string, log *slog.Logger) *Server {
	return &Server{ctrl: ctrl, tokenHash: sha256.Sum256([]byte(token)), version: version, log: log}
}

// HTTPServer returns a server with conservative limits and timeouts.
func (s *Server) HTTPServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      pairingTimeout + 15*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle("GET /api/v1/status", s.auth(s.status))
	mux.Handle("POST /api/v1/light", s.auth(s.light))
	mux.Handle("PATCH /api/v1/settings", s.auth(s.settings))
	mux.Handle("POST /api/v1/automation/resume", s.auth(s.resume))
	mux.Handle("POST /api/v1/tv/pair", s.auth(s.pair))
	return securityHeaders(mux)
}

type statusResponse struct {
	controller.Status
	Version string `json:"version"`
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, statusResponse{Status: s.ctrl.Status(), Version: s.version})
}

func (s *Server) light(w http.ResponseWriter, r *http.Request) {
	var req controller.LightRequest
	if !decode(w, r, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.ctrl.SetLight(r.Context(), req); err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, controller.ErrNotReady) {
			status = http.StatusServiceUnavailable
		}
		writeError(w, status, err.Error())
		return
	}
	s.status(w, r)
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	var patch controller.SettingsPatch
	if !decode(w, r, &patch) {
		return
	}
	if _, err := s.ctrl.UpdateSettings(patch); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.status(w, r)
}

func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	s.ctrl.ResumeAutomation()
	s.status(w, r)
}

func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Host string `json:"host"`
	}
	if r.ContentLength != 0 && !decode(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), pairingTimeout)
	defer cancel()
	if err := s.ctrl.PairTV(ctx, req.Host); err != nil {
		s.log.Warn("TV pairing failed", "error", err)
		msg := err.Error()
		if errors.Is(err, context.DeadlineExceeded) {
			msg = "No answer from the TV. Is it switched on, and did you accept the prompt?"
		}
		writeError(w, http.StatusBadGateway, msg)
		return
	}
	s.status(w, r)
}

// MARK: Auth

func (s *Server) auth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if s.failures.blocked(ip) {
			writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
			return
		}
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		// Compare hashes so neither the content nor the length of the token leaks through timing.
		sum := sha256.Sum256([]byte(token))
		if !ok || subtle.ConstantTimeCompare(sum[:], s.tokenHash[:]) != 1 {
			s.failures.fail(ip)
			s.log.Warn("rejected request with invalid token", "remote", ip, "path", r.URL.Path)
			w.Header().Set("WWW-Authenticate", `Bearer realm="ambientlightcontrol"`)
			writeError(w, http.StatusUnauthorized, "invalid or missing token")
			return
		}
		s.failures.reset(ip)
		next(w, r)
	})
}

func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return strings.Trim(host, "[]")
}

// failureLimiter blocks an address for a while after repeated wrong tokens.
type failureLimiter struct {
	mu      sync.Mutex
	entries map[string]*failureEntry
}

type failureEntry struct {
	count int
	until time.Time
}

const (
	maxFailures  = 10
	blockedFor   = 5 * time.Minute
	maxTrackedIP = 1024
)

func (l *failureLimiter) blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[ip]
	return e != nil && time.Now().Before(e.until)
}

func (l *failureLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.entries == nil || len(l.entries) > maxTrackedIP {
		l.entries = map[string]*failureEntry{}
	}
	e := l.entries[ip]
	if e == nil {
		e = &failureEntry{}
		l.entries[ip] = e
	}
	e.count++
	if e.count >= maxFailures {
		e.until = time.Now().Add(blockedFor)
		e.count = 0
	}
}

func (l *failureLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, ip)
}

// MARK: Helpers

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
