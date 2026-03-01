package envsync

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoginFailsForExpiredOrRevokedPAT(t *testing.T) {
	tests := []struct {
		name           string
		responseBody   string
		expectedPhrase string
	}{
		{name: "expired", responseBody: `{"error":"unauthorized","message":"token is expired"}`, expectedPhrase: "token is expired"},
		{name: "revoked", responseBody: `{"error":"unauthorized","message":"token is revoked"}`, expectedPhrase: "token is revoked"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/me" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer srv.Close()

			t.Setenv("ENVSYNC_CLOUD_ACCESS_TOKEN", "pat-token")
			app := &App{
				ConfigDir:   filepath.Join(t.TempDir(), "cfg"),
				SessionPath: filepath.Join(t.TempDir(), "cfg", "session.json"),
				CloudURL:    srv.URL,
				Stdin:       strings.NewReader(""),
				Stdout:      &bytes.Buffer{},
				Stderr:      &bytes.Buffer{},
				HTTPClient:  srv.Client(),
				Now:         time.Now,
			}

			err := app.Login()
			if err == nil {
				t.Fatalf("expected login error")
			}
			if !strings.Contains(err.Error(), tt.expectedPhrase) {
				t.Fatalf("expected %q in error, got %v", tt.expectedPhrase, err)
			}
		})
	}
}

func TestPullRejectsMismatchedRecoveryMetadata(t *testing.T) {
	tmp := t.TempDir()
	cwd := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout := &bytes.Buffer{}
	app := &App{
		ConfigDir:  filepath.Join(tmp, "cfg"),
		StatePath:  filepath.Join(tmp, "cfg", "state.json"),
		RemotePath: filepath.Join(tmp, "cfg", "remote.json"),
		RemoteURL:  "",
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     &bytes.Buffer{},
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
	}
	if err := app.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := app.ProjectCreate("api"); err != nil {
		t.Fatalf("project create: %v", err)
	}
	if err := app.ProjectUse("api"); err != nil {
		t.Fatalf("project use: %v", err)
	}
	phrase := strings.TrimSpace(stdout.String())
	lines := strings.Split(phrase, "\n")
	t.Setenv("ENVSYNC_RECOVERY_PHRASE", lines[len(lines)-1])

	store := map[string]any{
		"version":       1,
		"revision":      1,
		"salt_b64":      "ZmFrZXNhbHQ=",
		"key_check_b64": "ZmFrZWNoZWNr",
		"projects":      map[string]any{},
		"teams":         map[string]any{},
	}
	raw, _ := json.Marshal(store)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/store" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	app.RemoteURL = srv.URL
	app.RemoteToken = "compat-token"
	app.HTTPClient = srv.Client()

	err := app.Pull(true)
	if err == nil {
		t.Fatal("expected pull to fail for mismatched recovery metadata")
	}
	if !strings.Contains(err.Error(), "different recovery phrase") {
		t.Fatalf("expected recovery mismatch error, got %v", err)
	}
}
