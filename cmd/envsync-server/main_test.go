package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTestServer(t *testing.T) (*server, http.Handler) {
	t.Helper()
	s := &server{
		storePath:  filepath.Join(t.TempDir(), "remote.json"),
		store:      map[string]any{"version": float64(1), "revision": float64(0), "projects": map[string]any{}},
		authMode:   "off",
		authHeader: "X-Auth-Request-User",
		limiter:    newRateLimiter(1000, 1000),
		metrics:    &serverMetrics{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/v1/store", s.handleStore)
	return s, s.withMiddleware(mux)
}

func TestMiddlewareSetsRequestID(t *testing.T) {
	_, handler := newTestServer(t)
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Result().Header.Get("X-Request-Id") == "" {
		t.Fatal("expected X-Request-Id header")
	}
}

func TestRateLimiterReturns429(t *testing.T) {
	s, handler := newTestServer(t)
	s.limiter = newRateLimiter(0.01, 1)

	r1 := httptest.NewRequest(http.MethodGet, "/v1/store", nil)
	r1.RemoteAddr = "10.0.0.1:1234"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request: want 200, got %d", w1.Code)
	}

	r2 := httptest.NewRequest(http.MethodGet, "/v1/store", nil)
	r2.RemoteAddr = "10.0.0.1:1235"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: want 429, got %d", w2.Code)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	_, handler := newTestServer(t)

	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	rm := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	wm := httptest.NewRecorder()
	handler.ServeHTTP(wm, rm)

	body, _ := io.ReadAll(wm.Result().Body)
	out := string(body)
	if !strings.Contains(out, "envsync_requests_total") {
		t.Fatalf("metrics body missing requests_total: %s", out)
	}
}

func TestStoreTokenAuthMode(t *testing.T) {
	s, handler := newTestServer(t)
	s.authMode = "token"
	s.token = "secret"

	r1 := httptest.NewRequest(http.MethodGet, "/v1/store", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)
	if w1.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized request: want 401, got %d", w1.Code)
	}

	r2 := httptest.NewRequest(http.MethodGet, "/v1/store", nil)
	r2.Header.Set("Authorization", "Bearer secret")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("authorized request: want 200, got %d", w2.Code)
	}
}

func TestStoreHeaderAuthMode(t *testing.T) {
	s, handler := newTestServer(t)
	s.authMode = "header"
	s.authHeader = "X-Auth-Request-User"
	s.authProxySecret = "proxy-secret"

	r1 := httptest.NewRequest(http.MethodGet, "/v1/store", nil)
	r1.Header.Set("X-Auth-Request-User", "alice")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)
	if w1.Code != http.StatusUnauthorized {
		t.Fatalf("missing proxy secret: want 401, got %d", w1.Code)
	}

	r2 := httptest.NewRequest(http.MethodGet, "/v1/store", nil)
	r2.Header.Set("X-Auth-Request-User", "alice")
	r2.Header.Set("X-Envsync-Proxy-Secret", "proxy-secret")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("header auth request: want 200, got %d", w2.Code)
	}
}

func TestStoreLoadConcurrency(t *testing.T) {
	s, handler := newTestServer(t)
	s.authMode = "off"

	var (
		wg      sync.WaitGroup
		success int
		failed  int
		mu      sync.Mutex
	)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payload := map[string]any{
				"version":  float64(1),
				"revision": float64(i + 1),
				"projects": map[string]any{"p": map[string]any{}},
			}
			body, _ := json.Marshal(payload)
			r := httptest.NewRequest(http.MethodPut, "/v1/store", bytes.NewReader(body))
			r.Header.Set("If-Match", "0")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			mu.Lock()
			defer mu.Unlock()
			if w.Code == http.StatusOK {
				success++
			} else if w.Code == http.StatusConflict {
				failed++
			}
		}(i)
	}
	wg.Wait()
	if success != 1 {
		t.Fatalf("expected exactly one successful concurrent write, got %d", success)
	}
	if failed != 19 {
		t.Fatalf("expected 19 revision conflicts, got %d", failed)
	}

	for i := 0; i < 50; i++ {
		r := httptest.NewRequest(http.MethodGet, "/v1/store", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("concurrent read check failed: status=%d", w.Code)
		}
	}
}
