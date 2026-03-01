package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type contextKey string

const requestIDKey contextKey = "request_id"

type server struct {
	storePath       string
	token           string
	authMode        string
	authHeader      string
	authProxySecret string
	mu              sync.RWMutex
	store           map[string]any
	limiter         *rateLimiter
	metrics         *serverMetrics
}

type serverMetrics struct {
	requestsTotal     atomic.Uint64
	requests2xx       atomic.Uint64
	requests4xx       atomic.Uint64
	requests5xx       atomic.Uint64
	rateLimitedTotal  atomic.Uint64
	unauthorizedTotal atomic.Uint64
}

type rateLimiter struct {
	mu         sync.Mutex
	ratePerSec float64
	capacity   float64
	buckets    map[string]*tokenBucket
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func main() {
	addr := os.Getenv("ENVSYNC_SERVER_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	storePath := os.Getenv("ENVSYNC_SERVER_STORE")
	if storePath == "" {
		storePath = "./remote_store.json"
	}

	rpm := getenvInt("ENVSYNC_SERVER_RATE_LIMIT_RPM", 240)
	burst := getenvInt("ENVSYNC_SERVER_RATE_LIMIT_BURST", 40)

	rps := 0.0
	if rpm > 0 {
		rps = float64(rpm) / 60.0
	}
	limiter := newRateLimiter(rps, float64(max(1, burst)))

	s := &server{
		storePath:       storePath,
		token:           os.Getenv("ENVSYNC_SERVER_TOKEN"),
		authMode:        authModeFromEnv(os.Getenv("ENVSYNC_SERVER_AUTH_MODE"), os.Getenv("ENVSYNC_SERVER_TOKEN")),
		authHeader:      authHeaderFromEnv(os.Getenv("ENVSYNC_SERVER_AUTH_HEADER")),
		authProxySecret: strings.TrimSpace(os.Getenv("ENVSYNC_SERVER_AUTH_PROXY_SECRET")),
		limiter:         limiter,
		metrics:         &serverMetrics{},
	}
	if err := s.load(); err != nil {
		log.Fatalf("load store: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/v1/store", s.handleStore)

	handler := s.withMiddleware(mux)

	log.Printf("envsync-server listening on %s", addr)
	log.Printf("store file: %s", storePath)
	if s.token != "" {
		log.Printf("token auth: enabled")
	}
	log.Printf("auth mode: %s", s.authMode)
	if s.authMode == "header" || s.authMode == "token_or_header" {
		log.Printf("auth header: %s", s.authHeader)
		if s.authProxySecret != "" {
			log.Printf("proxy secret: enabled")
		}
	}
	if rpm > 0 {
		log.Printf("rate limit: %d rpm, burst %d", rpm, burst)
	} else {
		log.Printf("rate limit: disabled")
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-shutdownSignal()
		log.Printf("shutdown signal received")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func shutdownSignal() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	return ch
}

func (s *server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := requestIDFrom(r)
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		r = r.WithContext(ctx)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		rec.Header().Set("X-Request-Id", requestID)

		start := time.Now()

		if r.URL.Path == "/v1/store" && s.limiter.enabled() {
			ip := clientIP(r)
			if !s.limiter.Allow(ip, time.Now()) {
				s.metrics.rateLimitedTotal.Add(1)
				http.Error(rec, "rate limit exceeded", http.StatusTooManyRequests)
				rec.Header().Set("Retry-After", "1")
				s.logRequest(r, rec.status, time.Since(start), requestID)
				s.metrics.recordStatus(rec.status)
				return
			}
		}

		next.ServeHTTP(rec, r)
		s.metrics.recordStatus(rec.status)
		s.logRequest(r, rec.status, time.Since(start), requestID)
	})
}

func (s *server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "envsync_requests_total %d\n", s.metrics.requestsTotal.Load())
	fmt.Fprintf(w, "envsync_requests_2xx_total %d\n", s.metrics.requests2xx.Load())
	fmt.Fprintf(w, "envsync_requests_4xx_total %d\n", s.metrics.requests4xx.Load())
	fmt.Fprintf(w, "envsync_requests_5xx_total %d\n", s.metrics.requests5xx.Load())
	fmt.Fprintf(w, "envsync_rate_limited_total %d\n", s.metrics.rateLimitedTotal.Load())
	fmt.Fprintf(w, "envsync_unauthorized_total %d\n", s.metrics.unauthorizedTotal.Load())
}

func (s *server) logRequest(r *http.Request, status int, dur time.Duration, requestID string) {
	log.Printf("request_id=%s method=%s path=%s status=%d duration_ms=%d remote=%s", requestID, r.Method, r.URL.Path, status, dur.Milliseconds(), clientIP(r))
}

func (m *serverMetrics) recordStatus(status int) {
	m.requestsTotal.Add(1)
	switch {
	case status >= 200 && status < 300:
		m.requests2xx.Add(1)
	case status >= 400 && status < 500:
		m.requests4xx.Add(1)
	case status >= 500:
		m.requests5xx.Add(1)
	}
}

func newRateLimiter(ratePerSec, capacity float64) *rateLimiter {
	if capacity <= 0 {
		capacity = 1
	}
	r := &rateLimiter{
		ratePerSec: math.Max(0, ratePerSec),
		capacity:   capacity,
		buckets:    map[string]*tokenBucket{},
	}
	if r.ratePerSec > 0 {
		go func() {
			for {
				time.Sleep(5 * time.Minute)
				r.mu.Lock()
				now := time.Now()
				for k, b := range r.buckets {
					if now.Sub(b.last).Seconds() > (r.capacity/r.ratePerSec)*2 {
						delete(r.buckets, k)
					}
				}
				r.mu.Unlock()
			}
		}()
	}
	return r
}

func (r *rateLimiter) enabled() bool {
	return r.ratePerSec > 0
}

func (r *rateLimiter) Allow(key string, now time.Time) bool {
	if !r.enabled() {
		return true
	}
	if key == "" {
		key = "unknown"
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	bucket := r.buckets[key]
	if bucket == nil {
		r.buckets[key] = &tokenBucket{tokens: r.capacity - 1, last: now}
		return true
	}

	elapsed := now.Sub(bucket.last).Seconds()
	if elapsed > 0 {
		bucket.tokens = math.Min(r.capacity, bucket.tokens+(elapsed*r.ratePerSec))
		bucket.last = now
	}
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens -= 1
	return true
}

func requestIDFrom(r *http.Request) string {
	if existing := strings.TrimSpace(r.Header.Get("X-Request-Id")); existing != "" {
		return existing
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

func clientIP(r *http.Request) string {
	if ff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); ff != "" {
		parts := strings.Split(ff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

func getenvInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func (s *server) handleStore(w http.ResponseWriter, r *http.Request) {
	if err := s.authorize(r); err != nil {
		s.metrics.unauthorizedTotal.Add(1)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		defer s.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.store)
	case http.MethodPut:
		var next map[string]any
		if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if _, ok := next["projects"]; !ok {
			http.Error(w, "missing projects", http.StatusBadRequest)
			return
		}
		expected := 0
		if match := r.Header.Get("If-Match"); match != "" {
			v, err := strconv.Atoi(match)
			if err != nil {
				http.Error(w, "invalid If-Match", http.StatusBadRequest)
				return
			}
			expected = v
		}
		s.mu.Lock()
		currentRevision := asInt(s.store["revision"])
		if currentRevision != expected {
			s.mu.Unlock()
			http.Error(w, fmt.Sprintf("revision conflict: expected %d, got %d", expected, currentRevision), http.StatusConflict)
			return
		}
		next["revision"] = float64(currentRevision + 1)
		s.store = next
		err := s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) authorize(r *http.Request) error {
	switch s.authMode {
	case "off":
		return nil
	case "token":
		if s.validToken(r) {
			return nil
		}
	case "header":
		if s.validHeaderAuth(r) {
			return nil
		}
	case "token_or_header":
		if s.validToken(r) || s.validHeaderAuth(r) {
			return nil
		}
	default:
		return errors.New("server auth misconfigured")
	}
	return errors.New("unauthorized")
}

func (s *server) validToken(r *http.Request) bool {
	if s.token == "" {
		return false
	}
	want := "Bearer " + s.token
	got := r.Header.Get("Authorization")
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (s *server) validHeaderAuth(r *http.Request) bool {
	if s.authProxySecret != "" {
		got := strings.TrimSpace(r.Header.Get("X-Envsync-Proxy-Secret"))
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.authProxySecret)) != 1 || got == "" {
			return false
		}
	}
	return strings.TrimSpace(r.Header.Get(s.authHeader)) != ""
}

func authModeFromEnv(rawMode, token string) string {
	mode := strings.TrimSpace(strings.ToLower(rawMode))
	switch mode {
	case "", "auto":
		if strings.TrimSpace(token) != "" {
			return "token"
		}
		return "off"
	case "off", "token", "header", "token_or_header":
		return mode
	default:
		return "off"
	}
}

func authHeaderFromEnv(raw string) string {
	header := strings.TrimSpace(raw)
	if header == "" {
		return "X-Auth-Request-User"
	}
	return header
}

func (s *server) load() error {
	b, err := os.ReadFile(s.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.store = map[string]any{"version": float64(1), "revision": float64(0), "projects": map[string]any{}}
			return s.saveLocked()
		}
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("parse store: %w", err)
	}
	if _, ok := m["projects"]; !ok {
		m["projects"] = map[string]any{}
	}
	if _, ok := m["revision"]; !ok {
		m["revision"] = float64(0)
	}
	s.store = m
	return nil
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

func (s *server) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.storePath), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.store, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.storePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.storePath)
}
