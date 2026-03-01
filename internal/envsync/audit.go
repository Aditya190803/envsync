package envsync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// logAudit writes an audit event to the audit log file. It accepts the loaded
// state to avoid redundant disk I/O (the caller already has it).
func (a *App) logAudit(action string, state *State, fields map[string]any) {
	if a.AuditPath == "" {
		return
	}
	event := map[string]any{
		"ts":     a.Now().UTC().Format(time.RFC3339),
		"action": action,
		"cwd":    a.CWD,
	}
	if state != nil {
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
	a.rotateAuditIfNeeded(int64(len(b) + 1))
	f, err := os.OpenFile(a.AuditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

func (a *App) rotateAuditIfNeeded(nextWriteBytes int64) {
	if a.AuditPath == "" {
		return
	}
	info, err := os.Stat(a.AuditPath)
	if err != nil {
		return
	}

	rotateBySize := a.AuditMaxBytes > 0 && info.Size()+nextWriteBytes > a.AuditMaxBytes
	rotateByAge := a.AuditMaxAge > 0 && time.Since(info.ModTime()) >= a.AuditMaxAge && info.Size() > 0
	if !rotateBySize && !rotateByAge {
		a.pruneAuditByAge()
		return
	}

	limit := max(1, a.AuditMaxFiles)
	_ = os.Remove(a.AuditPath + "." + strconv.Itoa(limit))
	for i := limit - 1; i >= 1; i-- {
		src := a.AuditPath + "." + strconv.Itoa(i)
		dst := a.AuditPath + "." + strconv.Itoa(i+1)
		_ = os.Rename(src, dst)
	}
	_ = os.Rename(a.AuditPath, a.AuditPath+".1")
	a.pruneAuditByAge()
}

func (a *App) pruneAuditByAge() {
	if a.AuditPath == "" || a.AuditMaxAgeDays <= 0 {
		return
	}
	matches, err := filepath.Glob(a.AuditPath + ".*")
	if err != nil {
		return
	}
	cutoff := a.Now().Add(-time.Duration(a.AuditMaxAgeDays) * 24 * time.Hour)
	for _, p := range matches {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(p)
		}
	}
}
