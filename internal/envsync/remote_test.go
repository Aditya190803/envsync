package envsync

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoadRemoteHTTPRetriesTransientStatus(t *testing.T) {
	tmp := t.TempDir()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"version":1,"revision":7,"projects":{}}`)
	}))
	defer srv.Close()

	app := &App{
		ConfigDir:       filepath.Join(tmp, "cfg"),
		StatePath:       filepath.Join(tmp, "cfg", "state.json"),
		RemotePath:      filepath.Join(tmp, "cfg", "remote.json"),
		RemoteURL:       srv.URL,
		RemoteRetryMax:  3,
		RemoteRetryBase: time.Millisecond,
		RemoteRetryMaxD: 2 * time.Millisecond,
		Sleep:           func(time.Duration) {},
		HTTPClient:      srv.Client(),
		Stdout:          &bytes.Buffer{},
		Stderr:          &bytes.Buffer{},
	}

	remote, err := app.loadRemoteHTTP()
	if err != nil {
		t.Fatalf("load remote with retries: %v", err)
	}
	if remote.Revision != 7 {
		t.Fatalf("expected revision 7, got %d", remote.Revision)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls.Load())
	}
}

func TestSaveRemoteHTTPRetriesTemporaryNetworkError(t *testing.T) {
	tmp := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/store" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("If-Match"); got != "0" {
			http.Error(w, "bad if-match", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	baseTransport, ok := srv.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: &flakyTransport{base: baseTransport},
	}

	app := &App{
		ConfigDir:       filepath.Join(tmp, "cfg"),
		StatePath:       filepath.Join(tmp, "cfg", "state.json"),
		RemotePath:      filepath.Join(tmp, "cfg", "remote.json"),
		RemoteURL:       srv.URL,
		RemoteRetryMax:  3,
		RemoteRetryBase: time.Millisecond,
		RemoteRetryMaxD: 2 * time.Millisecond,
		Sleep:           func(time.Duration) {},
		HTTPClient:      client,
		Stdout:          &bytes.Buffer{},
		Stderr:          &bytes.Buffer{},
	}

	remote := &RemoteStore{
		Version:  1,
		Projects: map[string]*Project{},
		Teams:    map[string]*Team{},
	}
	if err := app.saveRemoteHTTP(remote, 0); err != nil {
		t.Fatalf("save remote with retries: %v", err)
	}
	if remote.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", remote.Revision)
	}
}

type flakyTransport struct {
	base  http.RoundTripper
	calls int32
}

func (t *flakyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if atomic.AddInt32(&t.calls, 1) == 1 {
		return nil, &tempNetErr{msg: "temporary network issue"}
	}
	return t.base.RoundTrip(req)
}

type tempNetErr struct {
	msg string
}

func (e *tempNetErr) Error() string   { return e.msg }
func (e *tempNetErr) Timeout() bool   { return false }
func (e *tempNetErr) Temporary() bool { return true }

func TestIsRetryableNetworkError(t *testing.T) {
	if isRetryableNetworkError(errors.New("plain")) {
		t.Fatal("plain error should not be retryable")
	}
	err := &net.DNSError{IsTemporary: true, Err: "temporary"}
	if !isRetryableNetworkError(err) {
		t.Fatal("net error should be retryable")
	}
	if !isRetryableNetworkError(&tempNetErr{msg: "temp"}) {
		t.Fatal("temporary net error should be retryable")
	}
}

func TestLoadRemoteHTTPDoesNotRetryOnUnauthorized(t *testing.T) {
	tmp := t.TempDir()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	app := &App{
		ConfigDir:       filepath.Join(tmp, "cfg"),
		StatePath:       filepath.Join(tmp, "cfg", "state.json"),
		RemotePath:      filepath.Join(tmp, "cfg", "remote.json"),
		RemoteURL:       srv.URL,
		RemoteRetryMax:  5,
		RemoteRetryBase: time.Millisecond,
		RemoteRetryMaxD: 2 * time.Millisecond,
		Sleep:           func(time.Duration) {},
		HTTPClient:      srv.Client(),
		Stdout:          &bytes.Buffer{},
		Stderr:          &bytes.Buffer{},
	}

	_, err := app.loadRemoteHTTP()
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 in error, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected single attempt, got %d", calls.Load())
	}
}
