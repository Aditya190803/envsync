package envsync

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultEnv     = "dev"
	roleAdmin      = "admin"
	roleMaintainer = "maintainer"
	roleWriter     = "writer"
	roleReader     = "reader"
)

type App struct {
	ConfigDir       string
	StatePath       string
	RemotePath      string
	AuditPath       string
	SessionPath     string
	CloudURL        string
	RemoteMode      string
	RemoteURL       string
	RemoteToken     string
	RemoteRetryMax  int
	RemoteRetryBase time.Duration
	RemoteRetryMaxD time.Duration
	CWD             string
	Now             func() time.Time
	Sleep           func(time.Duration)
	HTTPClient      *http.Client
	Stdin           io.Reader
	Stdout          io.Writer
	Stderr          io.Writer
	KeychainService string
	SessionService  string
	AuditMaxBytes   int64
	AuditMaxFiles   int
	AuditMaxAgeDays int
	AuditMaxAge     time.Duration
	FixPermissions  bool
	phraseCache     string
}

type State struct {
	Version         int                 `json:"version"`
	DeviceID        string              `json:"device_id"`
	SaltB64         string              `json:"salt_b64"`
	KeyCheckB64     string              `json:"key_check_b64"`
	CurrentTeam     string              `json:"current_team"`
	CurrentProject  string              `json:"current_project"`
	CurrentEnv      string              `json:"current_env"`
	ProjectBindings map[string]string   `json:"project_bindings"`
	Teams           map[string]*Team    `json:"teams"`
	Projects        map[string]*Project `json:"projects"`
}

type Project struct {
	Name string          `json:"name"`
	Team string          `json:"team,omitempty"`
	Envs map[string]*Env `json:"envs"`
}

type Team struct {
	Name    string            `json:"name"`
	Members map[string]string `json:"members"`
}

type Env struct {
	Name string                   `json:"name"`
	Vars map[string]*SecretRecord `json:"vars"`
}

type SecretRecord struct {
	CurrentVersion          int             `json:"current_version"`
	LastSyncedRemoteVersion int             `json:"last_synced_remote_version"`
	Versions                []SecretVersion `json:"versions"`
}

type SecretVersion struct {
	Version   int    `json:"version"`
	NonceB64  string `json:"nonce_b64"`
	CipherB64 string `json:"cipher_b64"`
	Deleted   bool   `json:"deleted"`
	Rotated   bool   `json:"rotated,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	UpdatedAt string `json:"updated_at"`
	DeviceID  string `json:"device_id"`
	PlainHash string `json:"plain_hash"`
}

type RemoteStore struct {
	Version     int                 `json:"version"`
	Revision    int                 `json:"revision"`
	SaltB64     string              `json:"salt_b64,omitempty"`
	KeyCheckB64 string              `json:"key_check_b64,omitempty"`
	Teams       map[string]*Team    `json:"teams,omitempty"`
	Projects    map[string]*Project `json:"projects"`
}

func NewApp() (*App, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	base := filepath.Join(configDir, "envsync")
	remote := os.Getenv("ENVSYNC_REMOTE_FILE")
	if remote == "" {
		remote = filepath.Join(base, "remote_store.json")
	}
	remoteURL := os.Getenv("ENVSYNC_REMOTE_URL")
	remoteToken := os.Getenv("ENVSYNC_REMOTE_TOKEN")
	cloudURL := strings.TrimSuffix(strings.TrimSpace(os.Getenv("ENVSYNC_CLOUD_URL")), "/")
	if cloudURL == "" && strings.TrimSpace(remoteURL) != "" {
		cloudURL = strings.TrimSuffix(strings.TrimSpace(remoteURL), "/")
	}
	app := &App{
		ConfigDir:       base,
		StatePath:       filepath.Join(base, "state.json"),
		RemotePath:      remote,
		AuditPath:       filepath.Join(base, "audit.log"),
		SessionPath:     filepath.Join(base, "session.json"),
		CloudURL:        cloudURL,
		RemoteMode:      strings.ToLower(strings.TrimSpace(os.Getenv("ENVSYNC_REMOTE_MODE"))),
		RemoteURL:       strings.TrimSuffix(remoteURL, "/"),
		RemoteToken:     remoteToken,
		RemoteRetryMax:  max(1, getenvInt("ENVSYNC_REMOTE_RETRY_MAX_ATTEMPTS", 3)),
		RemoteRetryBase: getenvDuration("ENVSYNC_REMOTE_RETRY_BASE_DELAY", 200*time.Millisecond),
		RemoteRetryMaxD: getenvDuration("ENVSYNC_REMOTE_RETRY_MAX_DELAY", 2*time.Second),
		CWD:             cwd,
		Now:             time.Now,
		Sleep:           time.Sleep,
		HTTPClient:      &http.Client{Timeout: 10 * time.Second},
		Stdin:           os.Stdin,
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
		KeychainService: "envsync-recovery-phrase",
		SessionService:  "envsync-cloud-session",
		AuditMaxBytes:   getenvInt64("ENVSYNC_AUDIT_MAX_BYTES", 1024*1024),
		AuditMaxFiles:   max(1, getenvInt("ENVSYNC_AUDIT_MAX_FILES", 5)),
		AuditMaxAgeDays: max(0, getenvInt("ENVSYNC_AUDIT_RETENTION_DAYS", 30)),
		AuditMaxAge:     getenvDuration("ENVSYNC_AUDIT_ROTATE_INTERVAL", 24*time.Hour),
		FixPermissions:  getenvBool("ENVSYNC_FIX_PERMISSIONS", false),
	}
	app.warnLegacyRemoteConfig()
	for _, warning := range app.verifyAndOptionallyFixPermissions() {
		fmt.Fprintf(app.Stderr, "warning: %s\n", warning)
	}
	return app, nil
}

func (a *App) warnLegacyRemoteConfig() {
	if getenvBool("ENVSYNC_SUPPRESS_LEGACY_REMOTE_WARNING", false) {
		return
	}
	legacyURL := strings.TrimSpace(os.Getenv("ENVSYNC_REMOTE_URL"))
	legacyToken := strings.TrimSpace(os.Getenv("ENVSYNC_REMOTE_TOKEN"))
	if legacyURL == "" && legacyToken == "" {
		return
	}
	if strings.Contains(strings.ToLower(legacyURL), "workers.dev") {
		fmt.Fprintln(a.Stderr, "warning: legacy Cloudflare Worker remote detected (ENVSYNC_REMOTE_URL). This path is deprecated; prefer cloud mode via `envsync login`.")
		return
	}
	fmt.Fprintln(a.Stderr, "warning: legacy self-host remote env vars (ENVSYNC_REMOTE_URL/ENVSYNC_REMOTE_TOKEN) are set. Cloud mode via `envsync login` is the default onboarding path.")
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

func getenvInt64(name string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return v
}

func getenvBool(name string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func getenvDuration(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return v
}

func (a *App) Init() error {
	if _, err := os.Stat(a.StatePath); err == nil {
		return errors.New("already initialized")
	}
	if err := os.MkdirAll(a.ConfigDir, 0o700); err != nil {
		return err
	}
	phrase, err := generatePhrase(12)
	if err != nil {
		return err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key := deriveKey(phrase, salt)
	check := keyCheck(key)
	deviceID, err := randomHex(8)
	if err != nil {
		return err
	}
	state := &State{
		Version:         1,
		DeviceID:        deviceID,
		SaltB64:         base64.StdEncoding.EncodeToString(salt),
		KeyCheckB64:     base64.StdEncoding.EncodeToString(check),
		CurrentEnv:      defaultEnv,
		ProjectBindings: map[string]string{},
		Teams:           map[string]*Team{},
		Projects:        map[string]*Project{},
	}
	if err := a.saveState(state); err != nil {
		return err
	}
	a.logAudit("init", state, map[string]any{"device_id": deviceID})
	fmt.Fprintf(a.Stdout, "%s\n\n", cSuccess("envsync initialized"))
	fmt.Fprintf(a.Stdout, "Recovery phrase (save this now; it is not stored):\n%s\n", cBold(phrase))
	return nil
}

func (a *App) TeamCreate(name string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	if name == "" {
		return errors.New("team name required")
	}
	if _, ok := state.Teams[name]; ok {
		return fmt.Errorf("team %q already exists", name)
	}
	actor := a.actorID(state)
	state.Teams[name] = &Team{
		Name:    name,
		Members: map[string]string{actor: roleAdmin},
	}
	if state.CurrentTeam == "" {
		state.CurrentTeam = name
	}
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "%s %s\n", cSuccess("created team"), cBold(name))
	a.logAudit("team_create", state, map[string]any{"team": name, "actor": actor})
	return nil
}

func (a *App) TeamList() error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	if len(state.Teams) == 0 {
		fmt.Fprintln(a.Stdout, cDim("no teams"))
		return nil
	}
	names := make([]string, 0, len(state.Teams))
	for n := range state.Teams {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		marker := "  "
		if state.CurrentTeam == n {
			marker = cSuccess("* ")
		}
		fmt.Fprintf(a.Stdout, "%s%s\n", marker, n)
	}
	return nil
}

func (a *App) TeamUse(name string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	if _, ok := state.Teams[name]; !ok {
		return fmt.Errorf("unknown team %q", name)
	}
	role, err := teamRole(state, name, a.actorID(state))
	if err != nil {
		return err
	}
	if role == "" {
		return fmt.Errorf("actor is not a member of team %q", name)
	}
	state.CurrentTeam = name
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "%s %s\n", cSuccess("using team"), cBold(name))
	a.logAudit("team_use", state, map[string]any{"team": name})
	return nil
}

func (a *App) TeamAddMember(teamName, actor, role string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	if teamName == "" {
		teamName = state.CurrentTeam
	}
	if teamName == "" {
		return errors.New("team name required")
	}
	team := state.Teams[teamName]
	if team == nil {
		return fmt.Errorf("unknown team %q", teamName)
	}
	if actor == "" {
		return errors.New("actor required")
	}
	if !isValidRole(role) {
		return fmt.Errorf("invalid role %q", role)
	}
	current := a.actorID(state)
	if !hasRole(state, teamName, current, roleAdmin) {
		return errors.New("team admin role required")
	}
	if team.Members == nil {
		team.Members = map[string]string{}
	}
	team.Members[actor] = role
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "%s %s to team %s as %s\n", cSuccess("added"), cBold(actor), cBold(teamName), cInfo("%s", role))
	a.logAudit("team_add_member", state, map[string]any{"team": teamName, "member": actor, "role": role})
	return nil
}

func (a *App) TeamRemoveMember(teamName, actor string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	if teamName == "" {
		teamName = state.CurrentTeam
	}
	if teamName == "" {
		return errors.New("team name required")
	}
	team := state.Teams[teamName]
	if team == nil {
		return fmt.Errorf("unknown team %q", teamName)
	}
	if actor == "" {
		return errors.New("actor required")
	}
	current := a.actorID(state)
	if !hasRole(state, teamName, current, roleAdmin) {
		return errors.New("team admin role required")
	}
	if _, ok := team.Members[actor]; !ok {
		return fmt.Errorf("member %q not found in team %q", actor, teamName)
	}
	if actor == current {
		return errors.New("cannot remove yourself from the team")
	}
	delete(team.Members, actor)
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "%s %s from team %s\n", cSuccess("removed"), cBold(actor), cBold(teamName))
	a.logAudit("team_remove_member", state, map[string]any{"team": teamName, "member": actor})
	return nil
}

func (a *App) TeamListMembers(teamName string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	if teamName == "" {
		teamName = state.CurrentTeam
	}
	if teamName == "" {
		return errors.New("team name required")
	}
	team := state.Teams[teamName]
	if team == nil {
		return fmt.Errorf("unknown team %q", teamName)
	}
	if len(team.Members) == 0 {
		fmt.Fprintln(a.Stdout, cDim("no members"))
		return nil
	}
	keys := make([]string, 0, len(team.Members))
	for k := range team.Members {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(a.Stdout, "%s %s\n", cInfo("%s", team.Members[k]), k)
	}
	return nil
}

func (a *App) ProjectCreate(name string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	if name == "" {
		return errors.New("project name required")
	}
	if _, ok := state.Projects[name]; ok {
		return fmt.Errorf("project %q already exists", name)
	}
	project := &Project{Name: name, Envs: map[string]*Env{defaultEnv: {Name: defaultEnv, Vars: map[string]*SecretRecord{}}}}
	if state.CurrentTeam != "" {
		if !hasRole(state, state.CurrentTeam, a.actorID(state), roleAdmin, roleWriter) {
			return errors.New("writer/admin role required on current team")
		}
		project.Team = state.CurrentTeam
	}
	state.Projects[name] = project
	if state.CurrentProject == "" {
		state.CurrentProject = name
	}
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "%s %s\n", cSuccess("created project"), cBold(name))
	a.logAudit("project_create", state, map[string]any{"project": name})
	return nil
}

func (a *App) ProjectList() error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	if len(state.Projects) == 0 {
		fmt.Fprintln(a.Stdout, cDim("no projects"))
		return nil
	}
	names := make([]string, 0, len(state.Projects))
	for n := range state.Projects {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		marker := "  "
		if state.CurrentProject == n {
			marker = cSuccess("* ")
		}
		fmt.Fprintf(a.Stdout, "%s%s\n", marker, n)
	}
	return nil
}

func (a *App) ProjectUse(name string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	project, ok := state.Projects[name]
	if !ok {
		return fmt.Errorf("unknown project %q", name)
	}
	if err := a.requireProjectRole(state, project, roleAdmin, roleWriter, roleReader); err != nil {
		return err
	}
	state.CurrentProject = name
	state.ProjectBindings[a.CWD] = name
	if state.CurrentEnv == "" {
		state.CurrentEnv = defaultEnv
	}
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "%s %s\n", cSuccess("using project"), cBold(name))
	a.logAudit("project_use", state, map[string]any{"project": name})
	return nil
}

func (a *App) ProjectDelete(name string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	if name == "" {
		return errors.New("project name required")
	}
	project, ok := state.Projects[name]
	if !ok {
		return fmt.Errorf("unknown project %q", name)
	}
	if err := a.requireProjectRole(state, project, roleAdmin); err != nil {
		return err
	}
	delete(state.Projects, name)
	// Clean up project bindings referencing the deleted project.
	for k, v := range state.ProjectBindings {
		if v == name {
			delete(state.ProjectBindings, k)
		}
	}
	if state.CurrentProject == name {
		state.CurrentProject = ""
	}
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "%s %s\n", cSuccess("deleted project"), cBold(name))
	a.logAudit("project_delete", state, map[string]any{"project": name})
	return nil
}

func (a *App) EnvCreate(name string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	project, _, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, project, roleAdmin, roleWriter); err != nil {
		return err
	}
	if _, ok := project.Envs[name]; ok {
		return fmt.Errorf("environment %q already exists", name)
	}
	project.Envs[name] = &Env{Name: name, Vars: map[string]*SecretRecord{}}
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "%s %s\n", cSuccess("created environment"), cBold(name))
	a.logAudit("env_create", state, map[string]any{"env": name})
	return nil
}

func (a *App) EnvUse(name string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	project, _, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, project, roleAdmin, roleWriter, roleReader); err != nil {
		return err
	}
	if _, ok := project.Envs[name]; !ok {
		return fmt.Errorf("unknown environment %q", name)
	}
	state.CurrentEnv = name
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "%s %s\n", cSuccess("using environment"), cBold(name))
	a.logAudit("env_use", state, map[string]any{"env": name})
	return nil
}

func (a *App) EnvList() error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	project, _, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, project, roleAdmin, roleWriter, roleReader); err != nil {
		return err
	}
	if len(project.Envs) == 0 {
		fmt.Fprintln(a.Stdout, cDim("no environments"))
		return nil
	}
	names := make([]string, 0, len(project.Envs))
	for n := range project.Envs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		marker := "  "
		if state.CurrentEnv == n {
			marker = cSuccess("* ")
		}
		fmt.Fprintf(a.Stdout, "%s%s\n", marker, n)
	}
	return nil
}

func (a *App) Set(keyName, value, expiresAt string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	project, _, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, project, roleAdmin, roleWriter); err != nil {
		return err
	}
	env, err := currentEnv(state, a.CWD)
	if err != nil {
		return err
	}
	if env.Vars == nil {
		env.Vars = map[string]*SecretRecord{}
	}

	// Resolve expiresAt — accept RFC3339 or Go duration string.
	resolvedExpiry := ""
	if expiresAt != "" {
		if _, parseErr := time.Parse(time.RFC3339, expiresAt); parseErr == nil {
			resolvedExpiry = expiresAt
		} else if dur, durErr := time.ParseDuration(expiresAt); durErr == nil {
			resolvedExpiry = a.Now().Add(dur).UTC().Format(time.RFC3339)
		} else {
			return fmt.Errorf("invalid --expires-at value %q: must be RFC3339 or a Go duration (e.g. 24h)", expiresAt)
		}
	}

	rec := env.Vars[keyName]
	if rec == nil {
		rec = &SecretRecord{}
	}
	next, err := a.writeSecretVersion(state, rec, value, false, resolvedExpiry)
	if err != nil {
		return err
	}
	env.Vars[keyName] = rec
	if err := a.saveState(state); err != nil {
		return err
	}
	msg := cSuccess("set") + " " + cBold(keyName)
	if resolvedExpiry != "" {
		msg += " " + cDim("(expires "+resolvedExpiry+")")
	}
	fmt.Fprintln(a.Stdout, msg)
	a.logAudit("set", state, map[string]any{"key": keyName, "version": next, "expires_at": resolvedExpiry})
	return nil
}

func (a *App) Rotate(keyName, value string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	project, _, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, project, roleAdmin, roleWriter); err != nil {
		return err
	}
	env, err := currentEnv(state, a.CWD)
	if err != nil {
		return err
	}
	rec := env.Vars[keyName]
	if rec == nil {
		return fmt.Errorf("key %q not found", keyName)
	}
	next, err := a.writeSecretVersion(state, rec, value, true, "")
	if err != nil {
		return err
	}
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "%s %s\n", cSuccess("rotated"), cBold(keyName))
	a.logAudit("rotate", state, map[string]any{"key": keyName, "version": next})
	return nil
}

func (a *App) Get(keyName string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	secretKey, err := a.getSecretKey(state)
	if err != nil {
		return err
	}
	project, _, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, project, roleAdmin, roleWriter, roleReader); err != nil {
		return err
	}
	env, err := currentEnv(state, a.CWD)
	if err != nil {
		return err
	}
	rec := env.Vars[keyName]
	if rec == nil || len(rec.Versions) == 0 {
		return fmt.Errorf("key %q not found", keyName)
	}
	v := rec.Versions[len(rec.Versions)-1]
	if v.Deleted {
		return fmt.Errorf("key %q is deleted", keyName)
	}
	// Check expiry.
	if v.ExpiresAt != "" {
		exp, parseErr := time.Parse(time.RFC3339, v.ExpiresAt)
		if parseErr == nil && a.Now().After(exp) {
			return fmt.Errorf("key %q has expired (at %s)", keyName, v.ExpiresAt)
		}
	}
	value, err := decrypt(secretKey, v)
	if err != nil {
		return err
	}
	fmt.Fprintln(a.Stdout, value)
	return nil
}

func (a *App) Delete(keyName string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	project, _, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, project, roleAdmin, roleWriter); err != nil {
		return err
	}
	env, err := currentEnv(state, a.CWD)
	if err != nil {
		return err
	}
	rec := env.Vars[keyName]
	if rec == nil {
		return fmt.Errorf("key %q not found", keyName)
	}
	next := rec.CurrentVersion + 1
	rec.CurrentVersion = next
	rec.Versions = append(rec.Versions, SecretVersion{
		Version:   next,
		Deleted:   true,
		UpdatedAt: a.Now().UTC().Format(time.RFC3339),
		DeviceID:  state.DeviceID,
	})
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "%s %s\n", cSuccess("deleted"), cBold(keyName))
	a.logAudit("delete", state, map[string]any{"key": keyName, "version": next})
	return nil
}

func (a *App) List(showValues bool) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	project, _, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, project, roleAdmin, roleWriter, roleReader); err != nil {
		return err
	}
	env, err := currentEnv(state, a.CWD)
	if err != nil {
		return err
	}
	if len(env.Vars) == 0 {
		fmt.Fprintln(a.Stdout, cDim("no variables"))
		return nil
	}
	var secretKey []byte
	if showValues {
		secretKey, err = a.getSecretKey(state)
		if err != nil {
			return err
		}
	}
	keys := make([]string, 0, len(env.Vars))
	for k := range env.Vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rec := env.Vars[k]
		if len(rec.Versions) == 0 {
			continue
		}
		v := rec.Versions[len(rec.Versions)-1]
		if v.Deleted {
			continue
		}
		// Mark expired keys.
		expired := false
		if v.ExpiresAt != "" {
			exp, parseErr := time.Parse(time.RFC3339, v.ExpiresAt)
			if parseErr == nil && a.Now().After(exp) {
				expired = true
			}
		}
		suffix := ""
		if expired {
			suffix = " " + cWarn("[EXPIRED]")
		}
		if !showValues {
			fmt.Fprintf(a.Stdout, "%s=%s%s\n", cBold(k), cDim("******"), suffix)
			continue
		}
		value, err := decrypt(secretKey, v)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Stdout, "%s=%s%s\n", cBold(k), value, suffix)
	}
	return nil
}

func (a *App) Load() error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	secretKey, err := a.getSecretKey(state)
	if err != nil {
		return err
	}
	project, _, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, project, roleAdmin, roleWriter, roleReader); err != nil {
		return err
	}
	env, err := currentEnv(state, a.CWD)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(env.Vars))
	for k := range env.Vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rec := env.Vars[k]
		if len(rec.Versions) == 0 {
			continue
		}
		v := rec.Versions[len(rec.Versions)-1]
		if v.Deleted {
			continue
		}
		// Skip expired keys.
		if v.ExpiresAt != "" {
			exp, parseErr := time.Parse(time.RFC3339, v.ExpiresAt)
			if parseErr == nil && a.Now().After(exp) {
				continue
			}
		}
		value, err := decrypt(secretKey, v)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Stdout, "export %s=%q\n", k, value)
	}
	return nil
}

func (a *App) History(keyName string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	project, _, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, project, roleAdmin, roleWriter, roleReader); err != nil {
		return err
	}
	env, err := currentEnv(state, a.CWD)
	if err != nil {
		return err
	}
	rec := env.Vars[keyName]
	if rec == nil {
		return fmt.Errorf("key %q not found", keyName)
	}
	for _, v := range rec.Versions {
		status := cSuccess("active")
		if v.Deleted {
			status = cError("deleted")
		} else if v.Rotated {
			status = cWarn("rotated")
		}
		fmt.Fprintf(a.Stdout, "v%d %s %s %s\n", v.Version, status, cDim(v.UpdatedAt), cDim(v.DeviceID))
	}
	return nil
}

func (a *App) Rollback(keyName string, version int) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	project, _, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, project, roleAdmin, roleWriter); err != nil {
		return err
	}
	env, err := currentEnv(state, a.CWD)
	if err != nil {
		return err
	}
	rec := env.Vars[keyName]
	if rec == nil {
		return fmt.Errorf("key %q not found", keyName)
	}
	var target *SecretVersion
	for i := range rec.Versions {
		if rec.Versions[i].Version == version {
			target = &rec.Versions[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("version %d not found", version)
	}
	next := rec.CurrentVersion + 1
	rec.CurrentVersion = next
	rec.Versions = append(rec.Versions, SecretVersion{
		Version:   next,
		NonceB64:  target.NonceB64,
		CipherB64: target.CipherB64,
		Deleted:   target.Deleted,
		UpdatedAt: a.Now().UTC().Format(time.RFC3339),
		DeviceID:  state.DeviceID,
		PlainHash: target.PlainHash,
	})
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "%s %s to v%d\n", cSuccess("rolled back"), cBold(keyName), version)
	a.logAudit("rollback", state, map[string]any{"key": keyName, "version": version})
	return nil
}

func (a *App) Doctor() error {
	return a.runDoctor(false)
}

func (a *App) DoctorJSON() error {
	return a.runDoctor(true)
}

func (a *App) Restore() error {
	if _, err := os.Stat(a.StatePath); err == nil {
		return errors.New("state already exists; remove it before restore")
	}
	remote, err := a.loadRemoteStore()
	if err != nil {
		return err
	}
	if remote.SaltB64 == "" || remote.KeyCheckB64 == "" {
		return errors.New("remote store has no restore metadata; run push from an initialized device first")
	}
	phrase, err := a.readPhrase()
	if err != nil {
		return err
	}
	salt, err := base64.StdEncoding.DecodeString(remote.SaltB64)
	if err != nil {
		return fmt.Errorf("invalid remote salt: %w", err)
	}
	key := deriveKey(phrase, salt)
	expected, err := base64.StdEncoding.DecodeString(remote.KeyCheckB64)
	if err != nil {
		return fmt.Errorf("invalid remote key check: %w", err)
	}
	if !hmac.Equal(expected, keyCheck(key)) {
		return errors.New("invalid recovery phrase")
	}
	deviceID, err := randomHex(8)
	if err != nil {
		return err
	}
	projects := remote.Projects
	if projects == nil {
		projects = map[string]*Project{}
	}
	teams := remote.Teams
	if teams == nil {
		teams = map[string]*Team{}
	}
	state := &State{
		Version:         1,
		DeviceID:        deviceID,
		SaltB64:         remote.SaltB64,
		KeyCheckB64:     remote.KeyCheckB64,
		CurrentEnv:      defaultEnv,
		ProjectBindings: map[string]string{},
		Teams:           teams,
		Projects:        projects,
	}
	if len(projects) > 0 {
		names := make([]string, 0, len(projects))
		for name := range projects {
			names = append(names, name)
		}
		sort.Strings(names)
		state.CurrentProject = names[0]
		state.ProjectBindings[a.CWD] = names[0]
	}
	markSyncedVersions(state.Projects)
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintln(a.Stdout, cSuccess("restore complete"))
	a.logAudit("restore", state, map[string]any{"device_id": deviceID, "projects": len(state.Projects)})
	return nil
}

func (a *App) Push(force bool) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	proj, projName, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, proj, roleAdmin, roleWriter); err != nil {
		return err
	}
	envName := state.CurrentEnv
	if envName == "" {
		envName = defaultEnv
	}
	localEnv := proj.Envs[envName]
	if localEnv == nil {
		return fmt.Errorf("environment %q not found", envName)
	}
	remote, err := a.loadRemoteStore()
	if err != nil {
		return err
	}
	expectedRevision := remote.Revision
	if err := validateRemoteCrypto(state, remote); err != nil {
		return err
	}
	attachCryptoMetadata(state, remote)
	remote.Teams = cloneTeams(state.Teams)
	remoteProject := remote.Projects[projName]
	if remoteProject == nil {
		remoteProject = &Project{Name: projName, Envs: map[string]*Env{}}
		remote.Projects[projName] = remoteProject
	}
	remoteEnv := remoteProject.Envs[envName]
	if remoteEnv == nil {
		remoteEnv = &Env{Name: envName, Vars: map[string]*SecretRecord{}}
		remoteProject.Envs[envName] = remoteEnv
	}
	conflicts := []string{}
	for k, localRec := range localEnv.Vars {
		remoteRec := remoteEnv.Vars[k]
		remoteCurrent := 0
		if remoteRec != nil {
			remoteCurrent = remoteRec.CurrentVersion
		}
		if remoteCurrent > localRec.LastSyncedRemoteVersion && localRec.CurrentVersion > localRec.LastSyncedRemoteVersion {
			conflicts = append(conflicts, k)
			continue
		}
		if localRec.CurrentVersion >= remoteCurrent {
			copyRec := *localRec
			remoteEnv.Vars[k] = &copyRec
			localRec.LastSyncedRemoteVersion = localRec.CurrentVersion
		}
	}
	if len(conflicts) > 0 && !force {
		return fmt.Errorf("push conflicts for keys: %s (rerun with --force)", strings.Join(conflicts, ", "))
	}
	if len(conflicts) > 0 && force {
		for _, k := range conflicts {
			localRec := localEnv.Vars[k]
			copyRec := *localRec
			remoteEnv.Vars[k] = &copyRec
			localRec.LastSyncedRemoteVersion = localRec.CurrentVersion
		}
	}
	if err := a.saveRemoteStore(remote, expectedRevision); err != nil {
		return err
	}
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintln(a.Stdout, cSuccess("push complete"))
	a.logAudit("push", state, map[string]any{"project": projName, "env": envName, "force": force})
	return nil
}

func (a *App) Pull(forceRemote bool) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	proj, projName, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, proj, roleAdmin, roleWriter, roleReader); err != nil {
		return err
	}
	envName := state.CurrentEnv
	if envName == "" {
		envName = defaultEnv
	}
	if proj.Envs[envName] == nil {
		proj.Envs[envName] = &Env{Name: envName, Vars: map[string]*SecretRecord{}}
	}
	localEnv := proj.Envs[envName]
	remote, err := a.loadRemoteStore()
	if err != nil {
		return err
	}
	if len(remote.Teams) > 0 {
		state.Teams = cloneTeams(remote.Teams)
	}
	if err := validateRemoteCrypto(state, remote); err != nil {
		return err
	}
	remoteProject := remote.Projects[projName]
	if remoteProject == nil {
		fmt.Fprintln(a.Stdout, cDim("nothing to pull"))
		return nil
	}
	remoteEnv := remoteProject.Envs[envName]
	if remoteEnv == nil {
		fmt.Fprintln(a.Stdout, cDim("nothing to pull"))
		return nil
	}
	conflicts := []string{}
	for k, remoteRec := range remoteEnv.Vars {
		localRec := localEnv.Vars[k]
		if localRec == nil {
			copyRec := *remoteRec
			copyRec.LastSyncedRemoteVersion = remoteRec.CurrentVersion
			localEnv.Vars[k] = &copyRec
			continue
		}
		if remoteRec.CurrentVersion > localRec.LastSyncedRemoteVersion && localRec.CurrentVersion > localRec.LastSyncedRemoteVersion {
			conflicts = append(conflicts, k)
			continue
		}
		if remoteRec.CurrentVersion >= localRec.CurrentVersion || forceRemote {
			copyRec := *remoteRec
			copyRec.LastSyncedRemoteVersion = remoteRec.CurrentVersion
			localEnv.Vars[k] = &copyRec
		}
	}
	if len(conflicts) > 0 && !forceRemote {
		return fmt.Errorf("pull conflicts for keys: %s (rerun with --force-remote)", strings.Join(conflicts, ", "))
	}
	if len(conflicts) > 0 && forceRemote {
		for _, k := range conflicts {
			remoteRec := remoteEnv.Vars[k]
			copyRec := *remoteRec
			copyRec.LastSyncedRemoteVersion = remoteRec.CurrentVersion
			localEnv.Vars[k] = &copyRec
		}
	}
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintln(a.Stdout, cSuccess("pull complete"))
	a.logAudit("pull", state, map[string]any{"project": projName, "env": envName, "force_remote": forceRemote})
	return nil
}

func (a *App) Diff() error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	proj, projName, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, proj, roleAdmin, roleWriter, roleReader); err != nil {
		return err
	}
	envName := state.CurrentEnv
	if envName == "" {
		envName = defaultEnv
	}
	localEnv := proj.Envs[envName]
	if localEnv == nil {
		localEnv = &Env{Name: envName, Vars: map[string]*SecretRecord{}}
	}

	remote, err := a.loadRemoteStore()
	if err != nil {
		return err
	}
	remoteProject := remote.Projects[projName]
	var remoteEnv *Env
	if remoteProject != nil {
		remoteEnv = remoteProject.Envs[envName]
	}
	if remoteEnv == nil {
		remoteEnv = &Env{Name: envName, Vars: map[string]*SecretRecord{}}
	}

	// Gather all keys from both sides.
	allKeys := map[string]bool{}
	for k := range localEnv.Vars {
		allKeys[k] = true
	}
	for k := range remoteEnv.Vars {
		allKeys[k] = true
	}
	if len(allKeys) == 0 {
		fmt.Fprintln(a.Stdout, cDim("no differences"))
		return nil
	}

	sorted := make([]string, 0, len(allKeys))
	for k := range allKeys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	hasDiff := false
	for _, k := range sorted {
		localRec := localEnv.Vars[k]
		remoteRec := remoteEnv.Vars[k]

		localVer := 0
		localDeleted := false
		if localRec != nil {
			localVer = localRec.CurrentVersion
			if len(localRec.Versions) > 0 {
				localDeleted = localRec.Versions[len(localRec.Versions)-1].Deleted
			}
		}

		remoteVer := 0
		remoteDeleted := false
		if remoteRec != nil {
			remoteVer = remoteRec.CurrentVersion
			if len(remoteRec.Versions) > 0 {
				remoteDeleted = remoteRec.Versions[len(remoteRec.Versions)-1].Deleted
			}
		}

		if localVer == remoteVer && localDeleted == remoteDeleted {
			// Check hash too for content differences.
			if localRec != nil && remoteRec != nil &&
				len(localRec.Versions) > 0 && len(remoteRec.Versions) > 0 &&
				localRec.Versions[len(localRec.Versions)-1].PlainHash == remoteRec.Versions[len(remoteRec.Versions)-1].PlainHash {
				continue
			}
			if localRec == nil && remoteRec == nil {
				continue
			}
		}

		hasDiff = true

		switch {
		case localRec == nil || (localVer == 0 && !localDeleted):
			// Only exists on remote.
			fmt.Fprintf(a.Stdout, "  %s %s %s\n", cInfo("+remote"), cBold(k), cDim(fmt.Sprintf("(v%d)", remoteVer)))
		case remoteRec == nil || (remoteVer == 0 && !remoteDeleted):
			// Only exists locally.
			fmt.Fprintf(a.Stdout, "  %s  %s %s\n", cSuccess("+local"), cBold(k), cDim(fmt.Sprintf("(v%d)", localVer)))
		case localVer > remoteVer:
			fmt.Fprintf(a.Stdout, "  %s %s %s\n", cWarn("↑ ahead"), cBold(k), cDim(fmt.Sprintf("(local v%d > remote v%d)", localVer, remoteVer)))
		case remoteVer > localVer:
			fmt.Fprintf(a.Stdout, "  %s %s %s\n", cWarn("↓ behind"), cBold(k), cDim(fmt.Sprintf("(remote v%d > local v%d)", remoteVer, localVer)))
		default:
			// Same version but different content.
			fmt.Fprintf(a.Stdout, "  %s %s %s\n", cError("≠ differs"), cBold(k), cDim(fmt.Sprintf("(v%d)", localVer)))
		}
	}
	if !hasDiff {
		fmt.Fprintln(a.Stdout, cSuccess("local and remote are in sync"))
	}
	return nil
}

func (a *App) PhraseSave() error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	phrase, err := a.readPhrase()
	if err != nil {
		return err
	}
	salt, err := base64.StdEncoding.DecodeString(state.SaltB64)
	if err != nil {
		return err
	}
	expected, err := base64.StdEncoding.DecodeString(state.KeyCheckB64)
	if err != nil {
		return err
	}
	key := deriveKey(phrase, salt)
	if !hmac.Equal(expected, keyCheck(key)) {
		return errors.New("invalid recovery phrase")
	}
	if err := a.storePhraseKeychain(phrase); err != nil {
		return err
	}
	a.phraseCache = phrase
	fmt.Fprintln(a.Stdout, cSuccess("saved recovery phrase to keychain"))
	a.logAudit("phrase_save", state, map[string]any{"service": a.keychainServiceName()})
	return nil
}

func (a *App) PhraseClear() error {
	if err := a.clearPhraseKeychain(); err != nil {
		return err
	}
	a.phraseCache = ""
	fmt.Fprintln(a.Stdout, cSuccess("cleared recovery phrase from keychain"))
	// State may not exist so we don't pass it.
	a.logAudit("phrase_clear", nil, map[string]any{"service": a.keychainServiceName()})
	return nil
}

func (a *App) readPhrase() (string, error) {
	if phrase := strings.TrimSpace(os.Getenv("ENVSYNC_RECOVERY_PHRASE")); phrase != "" {
		return phrase, nil
	}
	if phrase, err := a.phraseFromKeychain(); err == nil && phrase != "" {
		return phrase, nil
	}
	fmt.Fprint(a.Stderr, "Recovery phrase: ")
	reader := bufio.NewReader(a.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	phrase := strings.TrimSpace(line)
	if phrase == "" {
		return "", errors.New("recovery phrase cannot be empty")
	}
	return phrase, nil
}

func (a *App) getSecretKey(state *State) ([]byte, error) {
	if a.phraseCache == "" {
		phrase, err := a.readPhrase()
		if err != nil {
			return nil, err
		}
		a.phraseCache = phrase
	}
	salt, err := base64.StdEncoding.DecodeString(state.SaltB64)
	if err != nil {
		return nil, err
	}
	key := deriveKey(a.phraseCache, salt)
	expected, err := base64.StdEncoding.DecodeString(state.KeyCheckB64)
	if err != nil {
		return nil, err
	}
	if !hmac.Equal(expected, keyCheck(key)) {
		return nil, errors.New("invalid recovery phrase")
	}
	return key, nil
}

func (a *App) writeSecretVersion(state *State, rec *SecretRecord, value string, rotated bool, expiresAt string) (int, error) {
	secretKey, err := a.getSecretKey(state)
	if err != nil {
		return 0, err
	}
	ct, nonce, hash, err := encrypt(secretKey, value)
	if err != nil {
		return 0, err
	}
	next := rec.CurrentVersion + 1
	rec.CurrentVersion = next
	rec.Versions = append(rec.Versions, SecretVersion{
		Version:   next,
		NonceB64:  base64.StdEncoding.EncodeToString(nonce),
		CipherB64: base64.StdEncoding.EncodeToString(ct),
		Deleted:   false,
		Rotated:   rotated,
		ExpiresAt: expiresAt,
		UpdatedAt: a.Now().UTC().Format(time.RFC3339),
		DeviceID:  state.DeviceID,
		PlainHash: hash,
	})
	return next, nil
}

func (a *App) ImportEnv(file string) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[0] == v[len(v)-1] {
			v = v[1 : len(v)-1]
		}
		if err := a.Set(k, v, ""); err != nil {
			return fmt.Errorf("failed to import %s: %w", k, err)
		}
		count++
	}
	fmt.Fprintf(a.Stdout, "%s %d variables from %s\n", cSuccess("imported"), count, cBold(file))
	return nil
}

func (a *App) ExportEnv(file string) error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	secretKey, err := a.getSecretKey(state)
	if err != nil {
		return err
	}
	project, _, err := currentProject(state, a.CWD)
	if err != nil {
		return err
	}
	if err := a.requireProjectRole(state, project, roleAdmin, roleWriter, roleReader); err != nil {
		return err
	}
	env, err := currentEnv(state, a.CWD)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	keys := make([]string, 0, len(env.Vars))
	for k := range env.Vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	exported := 0
	for _, k := range keys {
		rec := env.Vars[k]
		if len(rec.Versions) == 0 {
			continue
		}
		v := rec.Versions[len(rec.Versions)-1]
		if v.Deleted {
			continue
		}
		value, err := decrypt(secretKey, v)
		if err != nil {
			return err
		}
		if strings.ContainsAny(value, " \n\r\t\"'") {
			buf.WriteString(fmt.Sprintf("%s=%q\n", k, value))
		} else {
			buf.WriteString(fmt.Sprintf("%s=%s\n", k, value))
		}
		exported++
	}
	if err := os.WriteFile(file, buf.Bytes(), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "%s %d variables to %s\n", cSuccess("exported"), exported, cBold(file))
	return nil
}
