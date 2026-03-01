package envsync

import (
	"fmt"
	"os"
	"strings"
)

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
