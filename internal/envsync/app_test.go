package envsync

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	_ = os.Setenv("ENVSYNC_KEYCHAIN_SERVICE", "envsync-test-phrase-"+suffix)
	_ = os.Setenv("ENVSYNC_SESSION_SERVICE", "envsync-test-session-"+suffix)
	_ = os.Setenv("NO_COLOR", "1")
	os.Exit(m.Run())
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	ct, nonce, _, err := encrypt(key, "hello")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	v := SecretVersion{NonceB64: encode(nonce), CipherB64: encode(ct)}
	out, err := decrypt(key, v)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if out != "hello" {
		t.Fatalf("want hello, got %q", out)
	}
}

func TestInitSetGetRollback(t *testing.T) {
	tmp := t.TempDir()
	cwd := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := &App{
		ConfigDir:  filepath.Join(tmp, "cfg"),
		StatePath:  filepath.Join(tmp, "cfg", "state.json"),
		RemotePath: filepath.Join(tmp, "cfg", "remote.json"),
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     stderr,
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
	}

	if err := app.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	phrase := lines[len(lines)-1]
	t.Setenv("ENVSYNC_RECOVERY_PHRASE", phrase)

	if err := app.ProjectCreate("api"); err != nil {
		t.Fatalf("project create: %v", err)
	}
	if err := app.ProjectUse("api"); err != nil {
		t.Fatalf("project use: %v", err)
	}
	if err := app.Set("TOKEN", "abc", ""); err != nil {
		t.Fatalf("set: %v", err)
	}

	stdout.Reset()
	if err := app.Get("TOKEN"); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "abc" {
		t.Fatalf("want abc, got %q", got)
	}

	if err := app.Set("TOKEN", "def", ""); err != nil {
		t.Fatalf("set2: %v", err)
	}
	if err := app.Rollback("TOKEN", 1); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	stdout.Reset()
	if err := app.Get("TOKEN"); err != nil {
		t.Fatalf("get after rollback: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "abc" {
		t.Fatalf("want abc after rollback, got %q", got)
	}
}

func TestInitReRunUpgradesLegacyStateSchema(t *testing.T) {
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
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     &bytes.Buffer{},
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
	}

	if err := app.Init(); err != nil {
		t.Fatalf("initial init: %v", err)
	}

	b, err := os.ReadFile(app.StatePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var disk map[string]any
	if err := json.Unmarshal(b, &disk); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	disk["version"] = float64(1)
	disk["current_env"] = ""
	legacy, err := json.MarshalIndent(disk, "", "  ")
	if err != nil {
		t.Fatalf("encode legacy state: %v", err)
	}
	if err := os.WriteFile(app.StatePath, legacy, 0o600); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	stdout.Reset()
	if err := app.Init(); err != nil {
		t.Fatalf("re-run init: %v", err)
	}
	if out := stdout.String(); !strings.Contains(out, "v1 -> v2") {
		t.Fatalf("expected upgrade message, got %q", out)
	}

	state, err := app.loadState()
	if err != nil {
		t.Fatalf("load upgraded state: %v", err)
	}
	if state.Version != currentStateSchemaVersion {
		t.Fatalf("expected state version %d, got %d", currentStateSchemaVersion, state.Version)
	}
	if state.CurrentEnv != defaultEnv {
		t.Fatalf("expected current env %q, got %q", defaultEnv, state.CurrentEnv)
	}
}

func TestPushPullHTTPRemote(t *testing.T) {
	tmp := t.TempDir()
	cwd := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	var (
		mu    sync.Mutex
		store = &RemoteStore{Version: 1, Revision: 0, Projects: map[string]*Project{}}
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/v1/store" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			mu.Lock()
			defer mu.Unlock()
			_ = json.NewEncoder(w).Encode(store)
		case http.MethodPut:
			if got := r.Header.Get("If-Match"); got != "0" {
				http.Error(w, "bad If-Match", http.StatusConflict)
				return
			}
			var next RemoteStore
			if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if next.Revision != 1 {
				http.Error(w, "bad revision", http.StatusConflict)
				return
			}
			mu.Lock()
			store = &next
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := &App{
		ConfigDir:   filepath.Join(tmp, "cfg"),
		StatePath:   filepath.Join(tmp, "cfg", "state.json"),
		RemotePath:  filepath.Join(tmp, "cfg", "remote.json"),
		RemoteURL:   srv.URL,
		RemoteToken: "test-token",
		CWD:         cwd,
		Stdin:       strings.NewReader(""),
		Stdout:      stdout,
		Stderr:      stderr,
		Now:         func() time.Time { return time.Unix(0, 0).UTC() },
		HTTPClient:  srv.Client(),
		phraseCache: "",
	}

	if err := app.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	phrase := lines[len(lines)-1]
	t.Setenv("ENVSYNC_RECOVERY_PHRASE", phrase)

	if err := app.ProjectCreate("api"); err != nil {
		t.Fatalf("project create: %v", err)
	}
	if err := app.ProjectUse("api"); err != nil {
		t.Fatalf("project use: %v", err)
	}
	if err := app.Set("TOKEN", "abc", ""); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := app.Push(false); err != nil {
		t.Fatalf("push: %v", err)
	}

	mu.Lock()
	remoteValue := store.Projects["api"].Envs["dev"].Vars["TOKEN"].CurrentVersion
	mu.Unlock()
	if remoteValue != 1 {
		t.Fatalf("expected remote version 1, got %d", remoteValue)
	}

	mu.Lock()
	rec := store.Projects["api"].Envs["dev"].Vars["TOKEN"]
	next := rec.Versions[0]
	next.Version = 2
	rec.CurrentVersion = 2
	rec.Versions = append(rec.Versions, next)
	mu.Unlock()
	if err := app.Pull(true); err != nil {
		t.Fatalf("pull: %v", err)
	}
	state, err := app.loadState()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	localVersion := state.Projects["api"].Envs["dev"].Vars["TOKEN"].CurrentVersion
	if localVersion != 2 {
		t.Fatalf("expected local version 2 after pull, got %d", localVersion)
	}
}

func TestRestoreFromRemoteFile(t *testing.T) {
	tmp := t.TempDir()
	cwd := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout := &bytes.Buffer{}
	app := &App{
		ConfigDir:  filepath.Join(tmp, "cfg-a"),
		StatePath:  filepath.Join(tmp, "cfg-a", "state.json"),
		RemotePath: filepath.Join(tmp, "shared", "remote.json"),
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     &bytes.Buffer{},
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
	}

	if err := app.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	phrase := lines[len(lines)-1]
	t.Setenv("ENVSYNC_RECOVERY_PHRASE", phrase)

	if err := app.ProjectCreate("api"); err != nil {
		t.Fatalf("project create: %v", err)
	}
	if err := app.ProjectUse("api"); err != nil {
		t.Fatalf("project use: %v", err)
	}
	if err := app.Set("TOKEN", "abc", ""); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := app.Push(false); err != nil {
		t.Fatalf("push: %v", err)
	}

	restoreOut := &bytes.Buffer{}
	restore := &App{
		ConfigDir:  filepath.Join(tmp, "cfg-b"),
		StatePath:  filepath.Join(tmp, "cfg-b", "state.json"),
		RemotePath: filepath.Join(tmp, "shared", "remote.json"),
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     restoreOut,
		Stderr:     &bytes.Buffer{},
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
	}
	if err := restore.Restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if err := restore.Get("TOKEN"); err != nil {
		t.Fatalf("get after restore: %v", err)
	}
	if !strings.Contains(restoreOut.String(), "abc") {
		t.Fatalf("expected restored value in output, got %q", restoreOut.String())
	}
}

func TestDoctorFailsWhenNotInitialized(t *testing.T) {
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
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     &bytes.Buffer{},
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
	}
	err := app.Doctor()
	if err == nil {
		t.Fatal("expected doctor to fail")
	}
	if !strings.Contains(stdout.String(), "[FAIL] state:") {
		t.Fatalf("expected state failure in doctor output, got %q", stdout.String())
	}
}

func TestTeamRBACReaderCannotSet(t *testing.T) {
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
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     &bytes.Buffer{},
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
	}

	if err := app.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	phrase := lines[len(lines)-1]
	t.Setenv("ENVSYNC_RECOVERY_PHRASE", phrase)

	if err := app.TeamCreate("core"); err != nil {
		t.Fatalf("team create: %v", err)
	}
	if err := app.ProjectCreate("api"); err != nil {
		t.Fatalf("project create: %v", err)
	}
	if err := app.ProjectUse("api"); err != nil {
		t.Fatalf("project use: %v", err)
	}
	if err := app.Set("TOKEN", "abc", ""); err != nil {
		t.Fatalf("admin set: %v", err)
	}
	if err := app.TeamAddMember("core", "viewer", roleReader); err != nil {
		t.Fatalf("add member: %v", err)
	}

	t.Setenv("ENVSYNC_ACTOR", "viewer")
	if err := app.ProjectUse("api"); err != nil {
		t.Fatalf("viewer project use: %v", err)
	}
	if err := app.Set("TOKEN", "def", ""); err == nil {
		t.Fatal("expected viewer set to fail")
	}
}

func TestRotateFlow(t *testing.T) {
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
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     &bytes.Buffer{},
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
	}

	if err := app.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	phrase := lines[len(lines)-1]
	t.Setenv("ENVSYNC_RECOVERY_PHRASE", phrase)

	if err := app.ProjectCreate("api"); err != nil {
		t.Fatalf("project create: %v", err)
	}
	if err := app.ProjectUse("api"); err != nil {
		t.Fatalf("project use: %v", err)
	}
	if err := app.Set("TOKEN", "abc", ""); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := app.Rotate("TOKEN", "def"); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	stdout.Reset()
	if err := app.Get("TOKEN"); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "def" {
		t.Fatalf("want def, got %q", got)
	}
}

func TestTeamRBACPrivilegedOperations(t *testing.T) {
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
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     &bytes.Buffer{},
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
	}

	if err := app.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	phrase := lines[len(lines)-1]
	t.Setenv("ENVSYNC_RECOVERY_PHRASE", phrase)

	if err := app.TeamCreate("core"); err != nil {
		t.Fatalf("team create: %v", err)
	}
	if err := app.ProjectCreate("api"); err != nil {
		t.Fatalf("project create: %v", err)
	}
	if err := app.ProjectUse("api"); err != nil {
		t.Fatalf("project use: %v", err)
	}
	if err := app.Set("TOKEN", "abc", ""); err != nil {
		t.Fatalf("admin set: %v", err)
	}
	if err := app.Push(false); err != nil {
		t.Fatalf("admin push: %v", err)
	}
	if err := app.TeamAddMember("core", "viewer", roleReader); err != nil {
		t.Fatalf("add member: %v", err)
	}

	t.Setenv("ENVSYNC_ACTOR", "viewer")
	if err := app.ProjectUse("api"); err != nil {
		t.Fatalf("viewer project use: %v", err)
	}

	if err := app.EnvCreate("prod"); err == nil {
		t.Fatal("expected env create to fail for reader")
	}
	if err := app.Set("TOKEN", "def", ""); err == nil {
		t.Fatal("expected set to fail for reader")
	}
	if err := app.Rotate("TOKEN", "ghi"); err == nil {
		t.Fatal("expected rotate to fail for reader")
	}
	if err := app.Delete("TOKEN"); err == nil {
		t.Fatal("expected delete to fail for reader")
	}
	if err := app.Rollback("TOKEN", 1); err == nil {
		t.Fatal("expected rollback to fail for reader")
	}
	if err := app.Push(false); err == nil {
		t.Fatal("expected push to fail for reader")
	}

	if err := app.Get("TOKEN"); err != nil {
		t.Fatalf("reader get should succeed: %v", err)
	}
	if err := app.List(false); err != nil {
		t.Fatalf("reader list should succeed: %v", err)
	}
	if err := app.History("TOKEN"); err != nil {
		t.Fatalf("reader history should succeed: %v", err)
	}
	if err := app.Load(); err != nil {
		t.Fatalf("reader load should succeed: %v", err)
	}
	if err := app.Pull(false); err != nil {
		t.Fatalf("reader pull should succeed: %v", err)
	}
}

func TestAuditIncludesActorAndContext(t *testing.T) {
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
		AuditPath:  filepath.Join(tmp, "cfg", "audit.log"),
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     &bytes.Buffer{},
		Now:        func() time.Time { return time.Unix(1700000000, 0).UTC() },
	}

	if err := app.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	phrase := lines[len(lines)-1]
	t.Setenv("ENVSYNC_RECOVERY_PHRASE", phrase)
	t.Setenv("ENVSYNC_ACTOR", "alice")

	if err := app.ProjectCreate("api"); err != nil {
		t.Fatalf("project create: %v", err)
	}
	if err := app.ProjectUse("api"); err != nil {
		t.Fatalf("project use: %v", err)
	}
	if err := app.Set("TOKEN", "abc", ""); err != nil {
		t.Fatalf("set: %v", err)
	}

	rawAudit, err := os.ReadFile(app.AuditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	entries := strings.Split(strings.TrimSpace(string(rawAudit)), "\n")
	if len(entries) == 0 {
		t.Fatal("expected audit entries")
	}

	var event map[string]any
	if err := json.Unmarshal([]byte(entries[len(entries)-1]), &event); err != nil {
		t.Fatalf("decode audit json: %v", err)
	}

	if got, _ := event["action"].(string); got != "set" {
		t.Fatalf("expected action set, got %q", got)
	}
	if got, _ := event["actor"].(string); got != "alice" {
		t.Fatalf("expected actor alice, got %q", got)
	}
	if got, _ := event["project"].(string); got != "api" {
		t.Fatalf("expected project api, got %q", got)
	}
	if got, _ := event["environment"].(string); got != "dev" {
		t.Fatalf("expected environment dev, got %q", got)
	}
	if got, _ := event["cwd"].(string); got != cwd {
		t.Fatalf("expected cwd %q, got %q", cwd, got)
	}
}

func TestLoadStateUpgradeCompatibility(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "cfg")
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}

	legacy := []byte(`{
  "version": 1,
  "device_id": "dev1",
  "salt_b64": "c2FsdA==",
  "key_check_b64": "Y2hlY2s=",
  "current_project": "",
  "current_env": "dev"
}`)
	statePath := filepath.Join(cfg, "state.json")
	if err := os.WriteFile(statePath, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	app := &App{
		ConfigDir:  cfg,
		StatePath:  statePath,
		RemotePath: filepath.Join(cfg, "remote.json"),
		CWD:        tmp,
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		Stdin:      strings.NewReader(""),
		Now:        time.Now,
	}
	state, err := app.loadState()
	if err != nil {
		t.Fatalf("load legacy state: %v", err)
	}
	if state.ProjectBindings == nil || state.Teams == nil || state.Projects == nil {
		t.Fatal("expected missing legacy fields to be default-initialized")
	}
}

func TestRemoteFileOptimisticConcurrencyConflict(t *testing.T) {
	tmp := t.TempDir()
	cwd := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	sharedRemote := filepath.Join(tmp, "shared", "remote.json")
	stdout := &bytes.Buffer{}
	app := &App{
		ConfigDir:  filepath.Join(tmp, "cfg"),
		StatePath:  filepath.Join(tmp, "cfg", "state.json"),
		RemotePath: sharedRemote,
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     &bytes.Buffer{},
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
	}
	if err := app.Init(); err != nil {
		t.Fatalf("init app: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	phrase := lines[len(lines)-1]
	t.Setenv("ENVSYNC_RECOVERY_PHRASE", phrase)

	if err := app.ProjectCreate("api"); err != nil {
		t.Fatalf("project create: %v", err)
	}
	if err := app.ProjectUse("api"); err != nil {
		t.Fatalf("project use: %v", err)
	}
	if err := app.Set("TOKEN", "a1", ""); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := app.Push(false); err != nil {
		t.Fatalf("push: %v", err)
	}

	remoteStale, err := app.loadRemoteStore()
	if err != nil {
		t.Fatalf("load remote stale: %v", err)
	}
	expectedRevision := remoteStale.Revision
	if expectedRevision != 1 {
		t.Fatalf("expected revision 1, got %d", expectedRevision)
	}

	remoteFresh := *remoteStale
	remoteFresh.Version = 1
	if err := app.saveRemoteStore(&remoteFresh, expectedRevision); err != nil {
		t.Fatalf("save remote fresh: %v", err)
	}

	if err := app.saveRemoteStore(remoteStale, expectedRevision); err == nil {
		t.Fatal("expected stale revision save to fail")
	}
}

func TestBackupRestoreDisasterRecoveryRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	cwd := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	remotePath := filepath.Join(tmp, "shared", "remote.json")
	stdout := &bytes.Buffer{}
	app := &App{
		ConfigDir:  filepath.Join(tmp, "cfg-a"),
		StatePath:  filepath.Join(tmp, "cfg-a", "state.json"),
		RemotePath: remotePath,
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     &bytes.Buffer{},
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
	}
	if err := app.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	phrase := lines[len(lines)-1]
	t.Setenv("ENVSYNC_RECOVERY_PHRASE", phrase)
	if err := app.ProjectCreate("api"); err != nil {
		t.Fatalf("project create: %v", err)
	}
	if err := app.ProjectUse("api"); err != nil {
		t.Fatalf("project use: %v", err)
	}
	if err := app.Set("TOKEN", "recoverable", ""); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := app.Push(false); err != nil {
		t.Fatalf("push: %v", err)
	}

	backupPath := filepath.Join(tmp, "backups", "remote-backup.json")
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(remotePath)
	if err != nil {
		t.Fatalf("read remote: %v", err)
	}
	if err := os.WriteFile(backupPath, raw, 0o600); err != nil {
		t.Fatalf("write backup: %v", err)
	}

	if err := os.WriteFile(remotePath, []byte(`{"version":1,"revision":1,"projects":{}}`), 0o600); err != nil {
		t.Fatalf("simulate disaster overwrite: %v", err)
	}
	rawBackup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if err := os.WriteFile(remotePath, rawBackup, 0o600); err != nil {
		t.Fatalf("restore backup: %v", err)
	}

	restoreOut := &bytes.Buffer{}
	restored := &App{
		ConfigDir:  filepath.Join(tmp, "cfg-b"),
		StatePath:  filepath.Join(tmp, "cfg-b", "state.json"),
		RemotePath: remotePath,
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     restoreOut,
		Stderr:     &bytes.Buffer{},
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
	}
	if err := restored.Restore(); err != nil {
		t.Fatalf("restore from backup: %v", err)
	}
	if err := restored.ProjectUse("api"); err != nil {
		t.Fatalf("project use restored: %v", err)
	}
	if err := restored.Get("TOKEN"); err != nil {
		t.Fatalf("get restored token: %v", err)
	}
	if !strings.Contains(restoreOut.String(), "recoverable") {
		t.Fatalf("expected recovered value, got: %q", restoreOut.String())
	}
}

func TestDoctorIncludesActionableHints(t *testing.T) {
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
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     &bytes.Buffer{},
		Now:        time.Now,
	}
	if err := app.Doctor(); err == nil {
		t.Fatal("expected doctor to fail")
	}
	out := stdout.String()
	if !strings.Contains(out, "hint: initialize or restore first") {
		t.Fatalf("expected actionable hint in doctor output, got %q", out)
	}
}

func TestDoctorJSONOutput(t *testing.T) {
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
		CWD:        cwd,
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     &bytes.Buffer{},
		Now:        time.Now,
	}
	err := app.DoctorJSON()
	if err == nil {
		t.Fatal("expected doctor json to report failure")
	}
	var payload struct {
		OK     bool `json:"ok"`
		Checks []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"checks"`
	}
	if decErr := json.Unmarshal(stdout.Bytes(), &payload); decErr != nil {
		t.Fatalf("decode doctor json: %v", decErr)
	}
	if payload.OK {
		t.Fatal("expected ok=false")
	}
	if len(payload.Checks) == 0 {
		t.Fatal("expected checks in doctor json")
	}
}

func TestRemoteFileConcurrentSaveRace(t *testing.T) {
	tmp := t.TempDir()
	remotePath := filepath.Join(tmp, "shared", "remote.json")
	if err := os.MkdirAll(filepath.Dir(remotePath), 0o700); err != nil {
		t.Fatal(err)
	}
	initial := &RemoteStore{
		Version:  1,
		Revision: 1,
		Projects: map[string]*Project{},
		Teams:    map[string]*Team{},
	}
	raw, _ := json.Marshal(initial)
	if err := os.WriteFile(remotePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	appA := &App{RemotePath: remotePath}
	appB := &App{RemotePath: remotePath}

	remoteA, err := appA.loadRemoteStore()
	if err != nil {
		t.Fatalf("load A: %v", err)
	}
	remoteB, err := appB.loadRemoteStore()
	if err != nil {
		t.Fatalf("load B: %v", err)
	}
	expected := remoteA.Revision

	var okCount atomic.Int32
	var conflictCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := appA.saveRemoteStore(remoteA, expected); err != nil {
			if strings.Contains(err.Error(), "remote changed concurrently") {
				conflictCount.Add(1)
				return
			}
			t.Errorf("save A: %v", err)
			return
		}
		okCount.Add(1)
	}()
	go func() {
		defer wg.Done()
		if err := appB.saveRemoteStore(remoteB, expected); err != nil {
			if strings.Contains(err.Error(), "remote changed concurrently") {
				conflictCount.Add(1)
				return
			}
			t.Errorf("save B: %v", err)
			return
		}
		okCount.Add(1)
	}()
	wg.Wait()

	if okCount.Load() != 1 || conflictCount.Load() != 1 {
		t.Fatalf("expected one success and one conflict, got success=%d conflict=%d", okCount.Load(), conflictCount.Load())
	}

	final, err := appA.loadRemoteStore()
	if err != nil {
		t.Fatalf("load final: %v", err)
	}
	if final.Revision != expected+1 {
		t.Fatalf("expected final revision %d, got %d", expected+1, final.Revision)
	}
}

func TestPermissionAutoFix(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "cfg")
	if err := os.MkdirAll(cfg, 0o777); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(cfg, "state.json")
	remotePath := filepath.Join(cfg, "remote.json")
	auditPath := filepath.Join(cfg, "audit.log")
	if err := os.WriteFile(statePath, []byte("{}"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(remotePath, []byte("{}"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(auditPath, []byte(""), 0o666); err != nil {
		t.Fatal(err)
	}

	app := &App{
		ConfigDir:      cfg,
		StatePath:      statePath,
		RemotePath:     remotePath,
		AuditPath:      auditPath,
		FixPermissions: true,
	}
	fixed := app.verifyAndOptionallyFixPermissions()
	if len(fixed) == 0 {
		t.Fatal("expected permission fixes")
	}
	for _, p := range []string{statePath, remotePath, auditPath} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("expected %s mode 600, got %o", p, info.Mode().Perm())
		}
	}
}

func TestAuditRotationBySize(t *testing.T) {
	tmp := t.TempDir()
	auditPath := filepath.Join(tmp, "audit.log")
	app := &App{
		AuditPath:       auditPath,
		CWD:             tmp,
		Stdout:          &bytes.Buffer{},
		Stderr:          &bytes.Buffer{},
		Now:             time.Now,
		AuditMaxBytes:   128,
		AuditMaxFiles:   3,
		AuditMaxAge:     0,
		AuditMaxAgeDays: 0,
	}
	state := &State{DeviceID: "dev1"}
	for i := 0; i < 20; i++ {
		app.logAudit("set", state, map[string]any{"i": i, "msg": strings.Repeat("x", 20)})
	}
	if _, err := os.Stat(auditPath + ".1"); err != nil {
		t.Fatalf("expected rotated audit file: %v", err)
	}
}

func encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
