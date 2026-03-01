package envsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const currentStateSchemaVersion = 2

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
	_, _, _ = migrateStateSchema(&state)
	return &state, nil
}

func (a *App) stateVersionOnDisk() (int, error) {
	b, err := os.ReadFile(a.StatePath)
	if err != nil {
		return 0, err
	}
	var raw struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return 0, err
	}
	if raw.Version <= 0 {
		return 1, nil
	}
	return raw.Version, nil
}

func (a *App) saveState(state *State) error {
	if err := os.MkdirAll(filepath.Dir(a.StatePath), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := a.StatePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.StatePath)
}

func currentProject(state *State, cwd string) (*Project, string, error) {
	name := state.CurrentProject
	if name == "" {
		name = state.ProjectBindings[cwd]
	}
	// Auto-detect from .envsync.json
	if name == "" {
		name = detectProjectFromMarker(cwd, state)
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

// detectProjectFromMarker reads a .envsync.json marker file in the given
// directory (or any parent) and returns the project name it references, if
// that project exists in the current state.
func detectProjectFromMarker(cwd string, state *State) string {
	dir := cwd
	for {
		markerPath := filepath.Join(dir, ".envsync.json")
		b, err := os.ReadFile(markerPath)
		if err == nil {
			var marker struct {
				Project string `json:"project"`
			}
			if json.Unmarshal(b, &marker) == nil && marker.Project != "" {
				if _, ok := state.Projects[marker.Project]; ok {
					return marker.Project
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
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

func migrateStateSchema(state *State) (changed bool, fromVersion int, toVersion int) {
	if state == nil {
		return false, 0, 0
	}

	fromVersion = state.Version
	if state.Version <= 0 {
		state.Version = 1
		changed = true
	}

	if state.Version < 2 {
		if state.CurrentEnv == "" {
			state.CurrentEnv = defaultEnv
		}
		state.Version = 2
		changed = true
	}

	if state.ProjectBindings == nil {
		state.ProjectBindings = map[string]string{}
		changed = true
	}
	if state.Teams == nil {
		state.Teams = map[string]*Team{}
		changed = true
	}
	if state.Projects == nil {
		state.Projects = map[string]*Project{}
		changed = true
	}
	if state.CurrentEnv == "" {
		state.CurrentEnv = defaultEnv
		changed = true
	}

	for _, team := range state.Teams {
		if team == nil {
			continue
		}
		if team.Members == nil {
			team.Members = map[string]string{}
			changed = true
		}
	}

	for _, project := range state.Projects {
		if project == nil {
			continue
		}
		if project.Envs == nil {
			project.Envs = map[string]*Env{}
			changed = true
		}
		for _, env := range project.Envs {
			if env == nil {
				continue
			}
			if env.Vars == nil {
				env.Vars = map[string]*SecretRecord{}
				changed = true
			}
		}
	}

	if state.Version > currentStateSchemaVersion {
		toVersion = state.Version
		return changed, fromVersion, toVersion
	}
	if state.Version < currentStateSchemaVersion {
		state.Version = currentStateSchemaVersion
		changed = true
	}

	toVersion = state.Version
	return changed, fromVersion, toVersion
}
