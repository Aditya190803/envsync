package envsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type doctorCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Details string `json:"details"`
	Hint    string `json:"hint,omitempty"`
}

func (a *App) collectDoctorChecks() ([]doctorCheck, *State) {
	checks := []doctorCheck{}
	add := func(name string, ok bool, details, hint string) {
		checks = append(checks, doctorCheck{Name: name, OK: ok, Details: details, Hint: hint})
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

	mode := a.effectiveRemoteMode()
	target := a.RemotePath
	if mode == "http" {
		target = a.RemoteURL
	}
	if mode == "cloud" {
		target = a.cloudBaseURL()
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

	for _, issue := range a.permissionIssues() {
		add("permissions", false, issue, "set ENVSYNC_FIX_PERMISSIONS=true to auto-fix insecure file modes")
	}

	return checks, state
}

func (a *App) runDoctor(asJSON bool) error {
	checks, state := a.collectDoctorChecks()
	hasFail := false
	for _, c := range checks {
		if !c.OK {
			hasFail = true
			break
		}
	}

	if asJSON {
		payload := map[string]any{
			"ok":     !hasFail,
			"checks": checks,
		}
		if err := json.NewEncoder(a.Stdout).Encode(payload); err != nil {
			return err
		}
	} else {
		for _, c := range checks {
			status := cOK()
			if !c.OK {
				status = cFAIL()
			}
			fmt.Fprintf(a.Stdout, "[%s] %s: %s\n", status, cBold(c.Name), c.Details)
			if !c.OK && c.Hint != "" {
				fmt.Fprintf(a.Stdout, "      %s %s\n", cDim("hint:"), cInfo("%s", c.Hint))
			}
		}
	}

	if hasFail {
		return errors.New("doctor found issues")
	}
	if state != nil {
		a.logAudit("doctor", state, map[string]any{"status": "ok"})
	}
	return nil
}
