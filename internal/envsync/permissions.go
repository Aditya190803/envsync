package envsync

import (
	"fmt"
	"os"
)

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (a *App) verifyAndOptionallyFixPermissions() []string {
	issues := a.permissionIssues()
	if len(issues) == 0 || !a.FixPermissions {
		return issues
	}

	fixed := []string{}
	for _, target := range []struct {
		path string
		mode os.FileMode
	}{
		{a.ConfigDir, 0o700},
		{a.StatePath, 0o600},
		{a.RemotePath, 0o600},
		{a.AuditPath, 0o600},
	} {
		if target.path == "" {
			continue
		}
		info, err := os.Stat(target.path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			if err := os.Chmod(target.path, 0o700); err == nil {
				fixed = append(fixed, fmt.Sprintf("fixed permissions for %s to 700", target.path))
			}
			continue
		}
		if err := os.Chmod(target.path, target.mode); err == nil {
			fixed = append(fixed, fmt.Sprintf("fixed permissions for %s to %o", target.path, target.mode))
		}
	}
	return fixed
}

func (a *App) permissionIssues() []string {
	issues := []string{}
	check := func(path string, expected os.FileMode, kind string) {
		if path == "" {
			return
		}
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return
			}
			issues = append(issues, fmt.Sprintf("%s %s unreadable: %v", kind, path, err))
			return
		}
		perms := info.Mode().Perm()
		if perms&^expected != 0 {
			issues = append(issues, fmt.Sprintf("%s %s is too permissive (%o, expected %o or stricter)", kind, path, perms, expected))
		}
	}

	check(a.ConfigDir, 0o700, "directory")
	check(a.StatePath, 0o600, "file")
	if a.RemoteURL == "" {
		check(a.RemotePath, 0o600, "file")
	}
	check(a.AuditPath, 0o600, "file")

	// Also check temp lock files in config dir if present.
	lockPath := a.RemotePath + ".lock"
	if _, err := os.Stat(lockPath); err == nil {
		check(lockPath, 0o600, "file")
	}
	return issues
}
