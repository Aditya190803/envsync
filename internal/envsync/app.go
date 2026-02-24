package envsync

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
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
	RemoteURL       string
	RemoteToken     string
	ConvexURL       string
	ConvexAPIKey    string
	ConvexDeployKey string
	ConvexGetPath   string
	ConvexPutPath   string
	CWD             string
	Now             func() time.Time
	HTTPClient      *http.Client
	Stdin           io.Reader
	Stdout          io.Writer
	Stderr          io.Writer
	KeychainService string
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
	convexURL := os.Getenv("ENVSYNC_CONVEX_URL")
	convexGetPath := os.Getenv("ENVSYNC_CONVEX_GET_PATH")
	if convexGetPath == "" {
		convexGetPath = "backup:getStore"
	}
	convexPutPath := os.Getenv("ENVSYNC_CONVEX_PUT_PATH")
	if convexPutPath == "" {
		convexPutPath = "backup:putStore"
	}
	return &App{
		ConfigDir:       base,
		StatePath:       filepath.Join(base, "state.json"),
		RemotePath:      remote,
		AuditPath:       filepath.Join(base, "audit.log"),
		RemoteURL:       strings.TrimSuffix(remoteURL, "/"),
		RemoteToken:     remoteToken,
		ConvexURL:       strings.TrimSuffix(convexURL, "/"),
		ConvexAPIKey:    os.Getenv("ENVSYNC_CONVEX_API_KEY"),
		ConvexDeployKey: os.Getenv("ENVSYNC_CONVEX_DEPLOY_KEY"),
		ConvexGetPath:   convexGetPath,
		ConvexPutPath:   convexPutPath,
		CWD:             cwd,
		Now:             time.Now,
		HTTPClient:      &http.Client{Timeout: 10 * time.Second},
		Stdin:           os.Stdin,
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
		KeychainService: "envsync-recovery-phrase",
	}, nil
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
	a.logAudit("init", map[string]any{"device_id": deviceID})
	fmt.Fprintf(a.Stdout, "envsync initialized\n\n")
	fmt.Fprintf(a.Stdout, "Recovery phrase (save this now; it is not stored):\n%s\n", phrase)
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
	fmt.Fprintf(a.Stdout, "created team %s\n", name)
	a.logAudit("team_create", map[string]any{"team": name, "actor": actor})
	return nil
}

func (a *App) TeamList() error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	if len(state.Teams) == 0 {
		fmt.Fprintln(a.Stdout, "no teams")
		return nil
	}
	names := make([]string, 0, len(state.Teams))
	for n := range state.Teams {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		marker := " "
		if state.CurrentTeam == n {
			marker = "*"
		}
		fmt.Fprintf(a.Stdout, "%s %s\n", marker, n)
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
	fmt.Fprintf(a.Stdout, "using team %s\n", name)
	a.logAudit("team_use", map[string]any{"team": name})
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
	fmt.Fprintf(a.Stdout, "added %s to team %s as %s\n", actor, teamName, role)
	a.logAudit("team_add_member", map[string]any{"team": teamName, "member": actor, "role": role})
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
		fmt.Fprintln(a.Stdout, "no members")
		return nil
	}
	keys := make([]string, 0, len(team.Members))
	for k := range team.Members {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(a.Stdout, "%s %s\n", team.Members[k], k)
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
	fmt.Fprintf(a.Stdout, "created project %s\n", name)
	a.logAudit("project_create", map[string]any{"project": name})
	return nil
}

func (a *App) ProjectList() error {
	state, err := a.loadState()
	if err != nil {
		return err
	}
	if len(state.Projects) == 0 {
		fmt.Fprintln(a.Stdout, "no projects")
		return nil
	}
	names := make([]string, 0, len(state.Projects))
	for n := range state.Projects {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		marker := " "
		if state.CurrentProject == n {
			marker = "*"
		}
		fmt.Fprintf(a.Stdout, "%s %s\n", marker, n)
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
	fmt.Fprintf(a.Stdout, "using project %s\n", name)
	a.logAudit("project_use", map[string]any{"project": name})
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
	fmt.Fprintf(a.Stdout, "created environment %s\n", name)
	a.logAudit("env_create", map[string]any{"env": name})
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
	fmt.Fprintf(a.Stdout, "using environment %s\n", name)
	a.logAudit("env_use", map[string]any{"env": name})
	return nil
}

func (a *App) Set(keyName, value string) error {
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
	rec := env.Vars[keyName]
	if rec == nil {
		rec = &SecretRecord{}
	}
	next, err := a.writeSecretVersion(state, rec, value, false)
	if err != nil {
		return err
	}
	env.Vars[keyName] = rec
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "set %s\n", keyName)
	a.logAudit("set", map[string]any{"key": keyName, "version": next})
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
	next, err := a.writeSecretVersion(state, rec, value, true)
	if err != nil {
		return err
	}
	if err := a.saveState(state); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "rotated %s\n", keyName)
	a.logAudit("rotate", map[string]any{"key": keyName, "version": next})
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
	fmt.Fprintf(a.Stdout, "deleted %s\n", keyName)
	a.logAudit("delete", map[string]any{"key": keyName, "version": next})
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
		fmt.Fprintln(a.Stdout, "no variables")
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
		if !showValues {
			fmt.Fprintf(a.Stdout, "%s=******\n", k)
			continue
		}
		value, err := decrypt(secretKey, v)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Stdout, "%s=%s\n", k, value)
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
		status := "active"
		if v.Deleted {
			status = "deleted"
		} else if v.Rotated {
			status = "rotated"
		}
		fmt.Fprintf(a.Stdout, "v%d %s %s %s\n", v.Version, status, v.UpdatedAt, v.DeviceID)
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
	fmt.Fprintf(a.Stdout, "rolled back %s to v%d\n", keyName, version)
	a.logAudit("rollback", map[string]any{"key": keyName, "version": version})
	return nil
}

func (a *App) Doctor() error {
	type check struct {
		Name    string
		OK      bool
		Details string
		Hint    string
	}
	checks := []check{}
	add := func(name string, ok bool, details, hint string) {
		checks = append(checks, check{Name: name, OK: ok, Details: details, Hint: hint})
	}

	if _, err := os.Stat(a.ConfigDir); err == nil {
		add("config_dir", true, a.ConfigDir, "")
	} else {
		add("config_dir", false, err.Error(), "run `envsync init` to create local config and state")
	}

	state, err := a.loadState()
	if err != nil {
		add("state", false, err.Error(), "initialize or restore first: `envsync init` or `envsync restore`")
	} else {
		project, projectName, pErr := currentProject(state, a.CWD)
		if pErr != nil {
			add("active_project", false, pErr.Error(), "select a project with `envsync project use <name>` or create one with `envsync project create <name>`")
		} else {
			add("active_project", true, projectName, "")
			envName := state.CurrentEnv
			if envName == "" {
				envName = defaultEnv
			}
			if project.Envs[envName] == nil {
				add("active_env", false, fmt.Sprintf("environment %q missing", envName), "create/select an environment: `envsync env create <name>` then `envsync env use <name>`")
			} else {
				add("active_env", true, envName, "")
			}
		}
	}

	mode := "file"
	target := a.RemotePath
	if a.RemoteURL != "" {
		mode = "http"
		target = a.RemoteURL
	}
	if a.ConvexURL != "" {
		mode = "convex"
		target = a.ConvexURL
	}
	add("remote_mode", true, mode, "")
	add("remote_target", true, target, "")

	if _, err := a.loadRemoteStore(); err != nil {
		add("remote_read", false, err.Error(), "verify remote settings/token reachability and retry `envsync pull`")
	} else {
		add("remote_read", true, "ok", "")
	}

	if os.Getenv("ENVSYNC_RECOVERY_PHRASE") != "" {
		add("recovery_phrase", true, "available via ENVSYNC_RECOVERY_PHRASE", "")
	} else if phrase, err := a.phraseFromKeychain(); err == nil && strings.TrimSpace(phrase) != "" {
		add("recovery_phrase", true, "available via keychain", "")
	} else {
		add("recovery_phrase", false, "ENVSYNC_RECOVERY_PHRASE is not set and keychain phrase is unavailable", "set ENVSYNC_RECOVERY_PHRASE or run `envsync phrase save` to use keychain-backed recovery")
	}

	hasFail := false
	for _, c := range checks {
		status := "OK"
		if !c.OK {
			status = "FAIL"
			hasFail = true
		}
		fmt.Fprintf(a.Stdout, "[%s] %s: %s\n", status, c.Name, c.Details)
		if !c.OK && c.Hint != "" {
			fmt.Fprintf(a.Stdout, "      hint: %s\n", c.Hint)
		}
	}
	if hasFail {
		return errors.New("doctor found issues")
	}
	a.logAudit("doctor", map[string]any{"status": "ok"})
	return nil
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
	fmt.Fprintln(a.Stdout, "restore complete")
	a.logAudit("restore", map[string]any{"device_id": deviceID, "projects": len(state.Projects)})
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
	fmt.Fprintln(a.Stdout, "push complete")
	a.logAudit("push", map[string]any{"project": projName, "env": envName, "force": force})
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
		fmt.Fprintln(a.Stdout, "nothing to pull")
		return nil
	}
	remoteEnv := remoteProject.Envs[envName]
	if remoteEnv == nil {
		fmt.Fprintln(a.Stdout, "nothing to pull")
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
	fmt.Fprintln(a.Stdout, "pull complete")
	a.logAudit("pull", map[string]any{"project": projName, "env": envName, "force_remote": forceRemote})
	return nil
}

func (a *App) loadState() (*State, error) {
	b, err := os.ReadFile(a.StatePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("envsync is not initialized; run `envsync init`")
		}
		return nil, err
	}
	var state State
	if err := json.Unmarshal(b, &state); err != nil {
		return nil, err
	}
	if state.ProjectBindings == nil {
		state.ProjectBindings = map[string]string{}
	}
	if state.Teams == nil {
		state.Teams = map[string]*Team{}
	}
	if state.Projects == nil {
		state.Projects = map[string]*Project{}
	}
	return &state, nil
}

func (a *App) saveState(state *State) error {
	if err := os.MkdirAll(filepath.Dir(a.StatePath), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.StatePath, b, 0o600)
}

func (a *App) loadRemoteStore() (*RemoteStore, error) {
	if a.ConvexURL != "" {
		return a.loadRemoteConvex()
	}
	if a.RemoteURL != "" {
		return a.loadRemoteHTTP()
	}
	return a.loadRemoteFile()
}

func (a *App) saveRemoteStore(remote *RemoteStore, expectedRevision int) error {
	if a.ConvexURL != "" {
		return a.saveRemoteConvex(remote, expectedRevision)
	}
	if a.RemoteURL != "" {
		return a.saveRemoteHTTP(remote, expectedRevision)
	}
	return a.saveRemoteFile(remote, expectedRevision)
}

func (a *App) loadRemoteFile() (*RemoteStore, error) {
	b, err := os.ReadFile(a.RemotePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &RemoteStore{Version: 1, Revision: 0, Teams: map[string]*Team{}, Projects: map[string]*Project{}}, nil
		}
		return nil, err
	}
	var remote RemoteStore
	if err := json.Unmarshal(b, &remote); err != nil {
		return nil, err
	}
	if remote.Projects == nil {
		remote.Projects = map[string]*Project{}
	}
	if remote.Teams == nil {
		remote.Teams = map[string]*Team{}
	}
	return &remote, nil
}

func (a *App) saveRemoteFile(remote *RemoteStore, expectedRevision int) error {
	current, err := a.loadRemoteFile()
	if err != nil {
		return err
	}
	if current.Revision != expectedRevision {
		return fmt.Errorf("remote changed concurrently: expected revision %d, got %d", expectedRevision, current.Revision)
	}
	remote.Revision = current.Revision + 1
	if err := os.MkdirAll(filepath.Dir(a.RemotePath), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(remote, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.RemotePath, b, 0o600)
}

func (a *App) loadRemoteHTTP() (*RemoteStore, error) {
	req, err := http.NewRequest(http.MethodGet, a.RemoteURL+"/v1/store", nil)
	if err != nil {
		return nil, err
	}
	if a.RemoteToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.RemoteToken)
	}
	client := a.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("remote GET failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var remote RemoteStore
	if err := json.NewDecoder(resp.Body).Decode(&remote); err != nil {
		return nil, err
	}
	if remote.Projects == nil {
		remote.Projects = map[string]*Project{}
	}
	if remote.Teams == nil {
		remote.Teams = map[string]*Team{}
	}
	return &remote, nil
}

func (a *App) saveRemoteHTTP(remote *RemoteStore, expectedRevision int) error {
	remote.Revision = expectedRevision + 1
	body, err := json.Marshal(remote)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, a.RemoteURL+"/v1/store", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", strconv.Itoa(expectedRevision))
	if a.RemoteToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.RemoteToken)
	}
	client := a.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("remote PUT failed: %s %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	return nil
}

type convexResponse struct {
	Status       string          `json:"status"`
	Value        json.RawMessage `json:"value"`
	ErrorMessage string          `json:"errorMessage"`
}

func (a *App) loadRemoteConvex() (*RemoteStore, error) {
	args := map[string]any{}
	if a.ConvexAPIKey != "" {
		args["apiKey"] = a.ConvexAPIKey
	}
	raw, err := a.callConvexFunction("query", a.ConvexGetPath, args)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return &RemoteStore{Version: 1, Revision: 0, Teams: map[string]*Team{}, Projects: map[string]*Project{}}, nil
	}
	var remote RemoteStore
	if err := json.Unmarshal(raw, &remote); err != nil {
		return nil, fmt.Errorf("convex decode store: %w", err)
	}
	if remote.Projects == nil {
		remote.Projects = map[string]*Project{}
	}
	if remote.Teams == nil {
		remote.Teams = map[string]*Team{}
	}
	return &remote, nil
}

func (a *App) saveRemoteConvex(remote *RemoteStore, expectedRevision int) error {
	remote.Revision = expectedRevision + 1
	args := map[string]any{
		"store":            remote,
		"expectedRevision": expectedRevision,
	}
	if a.ConvexAPIKey != "" {
		args["apiKey"] = a.ConvexAPIKey
	}
	_, err := a.callConvexFunction("mutation", a.ConvexPutPath, args)
	return err
}

func (a *App) callConvexFunction(kind, path string, args map[string]any) (json.RawMessage, error) {
	endpoint := convexEndpoint(a.ConvexURL, kind)
	body, err := json.Marshal(map[string]any{
		"path":   path,
		"args":   args,
		"format": "json",
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.ConvexDeployKey != "" {
		req.Header.Set("Authorization", "Convex "+a.ConvexDeployKey)
	}
	client := a.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("convex %s failed: %s %s", kind, resp.Status, strings.TrimSpace(string(errBody)))
	}
	var out convexResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Status == "error" {
		if out.ErrorMessage == "" {
			out.ErrorMessage = "unknown convex error"
		}
		return nil, errors.New(out.ErrorMessage)
	}
	return out.Value, nil
}

func attachCryptoMetadata(state *State, remote *RemoteStore) {
	remote.SaltB64 = state.SaltB64
	remote.KeyCheckB64 = state.KeyCheckB64
}

func validateRemoteCrypto(state *State, remote *RemoteStore) error {
	if remote.SaltB64 == "" && remote.KeyCheckB64 == "" {
		return nil
	}
	if remote.SaltB64 != state.SaltB64 || remote.KeyCheckB64 != state.KeyCheckB64 {
		return errors.New("remote store is encrypted with a different recovery phrase")
	}
	return nil
}

func (a *App) writeSecretVersion(state *State, rec *SecretRecord, value string, rotated bool) (int, error) {
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
		UpdatedAt: a.Now().UTC().Format(time.RFC3339),
		DeviceID:  state.DeviceID,
		PlainHash: hash,
	})
	return next, nil
}

func markSyncedVersions(projects map[string]*Project) {
	for _, project := range projects {
		if project == nil {
			continue
		}
		for _, env := range project.Envs {
			if env == nil {
				continue
			}
			for _, rec := range env.Vars {
				if rec == nil {
					continue
				}
				rec.LastSyncedRemoteVersion = rec.CurrentVersion
			}
		}
	}
}

func cloneTeams(in map[string]*Team) map[string]*Team {
	out := map[string]*Team{}
	for k, team := range in {
		if team == nil {
			continue
		}
		c := &Team{Name: team.Name, Members: map[string]string{}}
		for actor, role := range team.Members {
			c.Members[actor] = role
		}
		out[k] = c
	}
	return out
}

func (a *App) actorID(state *State) string {
	if actor := strings.TrimSpace(os.Getenv("ENVSYNC_ACTOR")); actor != "" {
		return actor
	}
	return state.DeviceID
}

func isValidRole(role string) bool {
	switch role {
	case roleAdmin, roleMaintainer, roleWriter, roleReader:
		return true
	default:
		return false
	}
}

func teamRole(state *State, teamName, actor string) (string, error) {
	if teamName == "" {
		return "", nil
	}
	team := state.Teams[teamName]
	if team == nil {
		return "", fmt.Errorf("unknown team %q", teamName)
	}
	return team.Members[actor], nil
}

func hasRole(state *State, teamName, actor string, allowed ...string) bool {
	role, err := teamRole(state, teamName, actor)
	if err != nil || role == "" {
		return false
	}
	role = canonicalRole(role)
	if role == roleAdmin {
		return true
	}
	for _, a := range allowed {
		if canonicalRole(a) == role {
			return true
		}
	}
	return false
}

func canonicalRole(role string) string {
	if role == roleWriter {
		return roleMaintainer
	}
	return role
}

func (a *App) requireProjectRole(state *State, project *Project, allowed ...string) error {
	if project == nil || project.Team == "" {
		return nil
	}
	actor := a.actorID(state)
	if hasRole(state, project.Team, actor, allowed...) {
		return nil
	}
	return fmt.Errorf("insufficient permissions for team %q", project.Team)
}

func convexEndpoint(base, kind string) string {
	base = strings.TrimSuffix(base, "/")
	if strings.HasSuffix(base, "/api") {
		return base + "/" + kind
	}
	return base + "/api/" + kind
}

func (a *App) keychainServiceName() string {
	if s := strings.TrimSpace(os.Getenv("ENVSYNC_KEYCHAIN_SERVICE")); s != "" {
		return s
	}
	if s := strings.TrimSpace(a.KeychainService); s != "" {
		return s
	}
	return "envsync-recovery-phrase"
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
	fmt.Fprintln(a.Stdout, "saved recovery phrase to keychain")
	a.logAudit("phrase_save", map[string]any{"service": a.keychainServiceName()})
	return nil
}

func (a *App) PhraseClear() error {
	if err := a.clearPhraseKeychain(); err != nil {
		return err
	}
	a.phraseCache = ""
	fmt.Fprintln(a.Stdout, "cleared recovery phrase from keychain")
	a.logAudit("phrase_clear", map[string]any{"service": a.keychainServiceName()})
	return nil
}

func (a *App) phraseFromKeychain() (string, error) {
	service := a.keychainServiceName()
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("security", "find-generic-password", "-a", "envsync", "-s", service, "-w").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	case "linux":
		out, err := exec.Command("secret-tool", "lookup", "service", service, "account", "envsync").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	default:
		return "", errors.New("keychain not supported on this OS")
	}
}

func (a *App) storePhraseKeychain(phrase string) error {
	service := a.keychainServiceName()
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("security", "add-generic-password", "-a", "envsync", "-s", service, "-w", phrase, "-U")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("keychain save failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	case "linux":
		cmd := exec.Command("secret-tool", "store", "--label=envsync recovery phrase", "service", service, "account", "envsync")
		cmd.Stdin = strings.NewReader(phrase)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("keychain save failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return errors.New("keychain save not supported on this OS")
	}
}

func (a *App) clearPhraseKeychain() error {
	service := a.keychainServiceName()
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("security", "delete-generic-password", "-a", "envsync", "-s", service)
		if out, err := cmd.CombinedOutput(); err != nil {
			msg := strings.TrimSpace(string(out))
			if strings.Contains(strings.ToLower(msg), "could not be found") {
				return nil
			}
			return fmt.Errorf("keychain clear failed: %s", msg)
		}
		return nil
	case "linux":
		cmd := exec.Command("secret-tool", "clear", "service", service, "account", "envsync")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("keychain clear failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return errors.New("keychain clear not supported on this OS")
	}
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

func (a *App) logAudit(action string, fields map[string]any) {
	if a.AuditPath == "" {
		return
	}
	event := map[string]any{
		"ts":     a.Now().UTC().Format(time.RFC3339),
		"action": action,
		"cwd":    a.CWD,
	}
	if state, err := a.loadState(); err == nil {
		event["actor"] = a.actorID(state)
		event["device_id"] = state.DeviceID
		if state.CurrentTeam != "" {
			event["team"] = state.CurrentTeam
		}
		if state.CurrentProject != "" {
			event["project"] = state.CurrentProject
		}
		if state.CurrentEnv != "" {
			event["environment"] = state.CurrentEnv
		}
	}
	for k, v := range fields {
		event[k] = v
	}
	b, err := json.Marshal(event)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(a.AuditPath), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(a.AuditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

func deriveKey(phrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(phrase), salt, 1, 64*1024, 4, 32)
}

func keyCheck(key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte("envsync-key-check"))
	return h.Sum(nil)
}

func encrypt(key []byte, plaintext string) ([]byte, []byte, string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, "", err
	}
	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	h := sha256.Sum256([]byte(plaintext))
	return ct, nonce, hex.EncodeToString(h[:]), nil
}

func decrypt(key []byte, v SecretVersion) (string, error) {
	nonce, err := base64.StdEncoding.DecodeString(v.NonceB64)
	if err != nil {
		return "", err
	}
	ct, err := base64.StdEncoding.DecodeString(v.CipherB64)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func currentProject(state *State, cwd string) (*Project, string, error) {
	name := state.CurrentProject
	if name == "" {
		name = state.ProjectBindings[cwd]
	}
	if name == "" {
		return nil, "", errors.New("no active project; run `envsync project create <name>` and `envsync project use <name>`")
	}
	p := state.Projects[name]
	if p == nil {
		return nil, "", fmt.Errorf("active project %q missing", name)
	}
	if p.Envs == nil {
		p.Envs = map[string]*Env{}
	}
	return p, name, nil
}

func currentEnv(state *State, cwd string) (*Env, error) {
	p, _, err := currentProject(state, cwd)
	if err != nil {
		return nil, err
	}
	envName := state.CurrentEnv
	if envName == "" {
		envName = defaultEnv
	}
	env := p.Envs[envName]
	if env == nil {
		return nil, fmt.Errorf("environment %q does not exist", envName)
	}
	if env.Vars == nil {
		env.Vars = map[string]*SecretRecord{}
	}
	return env, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func generatePhrase(words int) (string, error) {
	parts := make([]string, words)
	for i := 0; i < words; i++ {
		idx, err := randomWordIndex(len(wordList))
		if err != nil {
			return "", err
		}
		parts[i] = wordList[idx]
	}
	return strings.Join(parts, " "), nil
}

func randomWordIndex(max int) (int, error) {
	if max <= 0 {
		return 0, errors.New("invalid word list")
	}
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		return 0, err
	}
	v := int(b[0])<<8 | int(b[1])
	return v % max, nil
}

var wordList = []string{
	"amber", "angle", "apple", "artist", "atom", "badge", "balance", "beam", "berry", "bird", "breeze", "brick", "cable", "cactus", "candle", "canvas", "carbon", "cedar", "charm", "circle", "cloud", "cobalt", "comet", "copper", "coral", "crystal", "delta", "drift", "eagle", "echo", "ember", "field", "flame", "forest", "fossil", "frost", "galaxy", "garden", "glacier", "gold", "granite", "harbor", "hazel", "horizon", "island", "jade", "jungle", "keystone", "lagoon", "lantern", "leaf", "lilac", "lunar", "maple", "marble", "meadow", "mercury", "meteor", "mist", "mountain", "nebula", "nectar", "oasis", "ocean", "onyx", "orchid", "pearl", "pepper", "phoenix", "pine", "planet", "plume", "polar", "prairie", "quartz", "raven", "river", "rocket", "sable", "saffron", "sage", "sand", "scarlet", "shadow", "silver", "solar", "spark", "spice", "spring", "stone", "storm", "summit", "sunrise", "teal", "thunder", "timber", "topaz", "valley", "velvet", "violet", "wave", "willow", "winter", "zephyr",
}
