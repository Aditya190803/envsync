package main

import (
	"bytes"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls  map[string]int
	lastKV map[string]string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		calls:  map[string]int{},
		lastKV: map[string]string{},
	}
}

func (f *fakeRunner) mark(name string) { f.calls[name]++ }

func (f *fakeRunner) Init() error                     { f.mark("Init"); return nil }
func (f *fakeRunner) Login() error                    { f.mark("Login"); return nil }
func (f *fakeRunner) Logout() error                   { f.mark("Logout"); return nil }
func (f *fakeRunner) WhoAmI() error                   { f.mark("WhoAmI"); return nil }
func (f *fakeRunner) ProjectCreate(name string) error { f.mark("ProjectCreate"); return nil }
func (f *fakeRunner) ProjectList() error              { f.mark("ProjectList"); return nil }
func (f *fakeRunner) ProjectUse(name string) error    { f.mark("ProjectUse"); return nil }
func (f *fakeRunner) ProjectDelete(name string) error { f.mark("ProjectDelete"); return nil }
func (f *fakeRunner) TeamCreate(name string) error    { f.mark("TeamCreate"); return nil }
func (f *fakeRunner) TeamList() error                 { f.mark("TeamList"); return nil }
func (f *fakeRunner) TeamUse(name string) error       { f.mark("TeamUse"); return nil }
func (f *fakeRunner) TeamAddMember(teamName, actor, role string) error {
	f.mark("TeamAddMember")
	return nil
}
func (f *fakeRunner) TeamRemoveMember(teamName, actor string) error {
	f.mark("TeamRemoveMember")
	return nil
}
func (f *fakeRunner) TeamListMembers(teamName string) error { f.mark("TeamListMembers"); return nil }
func (f *fakeRunner) EnvCreate(name string) error           { f.mark("EnvCreate"); return nil }
func (f *fakeRunner) EnvUse(name string) error              { f.mark("EnvUse"); return nil }
func (f *fakeRunner) EnvList() error                        { f.mark("EnvList"); return nil }
func (f *fakeRunner) Rotate(keyName, value string) error    { f.mark("Rotate"); return nil }
func (f *fakeRunner) Get(keyName string) error              { f.mark("Get"); return nil }
func (f *fakeRunner) Delete(keyName string) error           { f.mark("Delete"); return nil }
func (f *fakeRunner) List(showValues bool) error            { f.mark("List"); return nil }
func (f *fakeRunner) Load() error                           { f.mark("Load"); return nil }
func (f *fakeRunner) ImportEnv(file string) error           { f.mark("ImportEnv"); return nil }
func (f *fakeRunner) ExportEnv(file string) error           { f.mark("ExportEnv"); return nil }
func (f *fakeRunner) History(keyName string) error          { f.mark("History"); return nil }
func (f *fakeRunner) Diff() error                           { f.mark("Diff"); return nil }
func (f *fakeRunner) PhraseSave() error                     { f.mark("PhraseSave"); return nil }
func (f *fakeRunner) PhraseClear() error                    { f.mark("PhraseClear"); return nil }
func (f *fakeRunner) Doctor() error                         { f.mark("Doctor"); return nil }
func (f *fakeRunner) DoctorJSON() error                     { f.mark("DoctorJSON"); return nil }
func (f *fakeRunner) Restore() error                        { f.mark("Restore"); return nil }
func (f *fakeRunner) Set(keyName, value, expiresAt string) error {
	f.mark("Set")
	f.lastKV["key"] = keyName
	f.lastKV["value"] = value
	f.lastKV["expiresAt"] = expiresAt
	return nil
}
func (f *fakeRunner) Rollback(keyName string, version int) error {
	f.mark("Rollback")
	f.lastKV["key"] = keyName
	f.lastKV["version"] = "set"
	return nil
}
func (f *fakeRunner) Push(force bool) error {
	f.mark("Push")
	if force {
		f.lastKV["force"] = "true"
	}
	return nil
}
func (f *fakeRunner) Pull(forceRemote bool) error {
	f.mark("Pull")
	if forceRemote {
		f.lastKV["force_remote"] = "true"
	}
	return nil
}

func TestRollbackRequiresVersionFlag(t *testing.T) {
	r := newFakeRunner()
	buf := &bytes.Buffer{}
	cmd := buildRootCmd(r, buf)
	cmd.SetArgs([]string{"rollback", "API_KEY"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--version is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRollbackRejectsInvalidVersion(t *testing.T) {
	r := newFakeRunner()
	buf := &bytes.Buffer{}
	cmd := buildRootCmd(r, buf)
	cmd.SetArgs([]string{"rollback", "API_KEY", "--version", "abc"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetPushPullRestoreWiring(t *testing.T) {
	r := newFakeRunner()
	buf := &bytes.Buffer{}

	tests := []struct {
		args []string
		call string
	}{
		{[]string{"set", "API_KEY", "secret", "--expires-at", "24h"}, "Set"},
		{[]string{"push", "--force"}, "Push"},
		{[]string{"pull", "--force-remote"}, "Pull"},
		{[]string{"restore"}, "Restore"},
	}

	for _, tc := range tests {
		cmd := buildRootCmd(r, buf)
		cmd.SetArgs(tc.args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", tc.args, err)
		}
		if r.calls[tc.call] == 0 {
			t.Fatalf("expected %s to be called", tc.call)
		}
	}

	if got := r.lastKV["expiresAt"]; got != "24h" {
		t.Fatalf("expected expires-at 24h, got %q", got)
	}
	if r.lastKV["force"] != "true" {
		t.Fatalf("expected force flag to be wired")
	}
	if r.lastKV["force_remote"] != "true" {
		t.Fatalf("expected force-remote flag to be wired")
	}
}

func TestDoctorJSONWiring(t *testing.T) {
	r := newFakeRunner()
	buf := &bytes.Buffer{}
	cmd := buildRootCmd(r, buf)
	cmd.SetArgs([]string{"doctor", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor --json failed: %v", err)
	}
	if r.calls["DoctorJSON"] != 1 {
		t.Fatalf("expected DoctorJSON call, got %d", r.calls["DoctorJSON"])
	}
	if r.calls["Doctor"] != 0 {
		t.Fatalf("expected Doctor not called, got %d", r.calls["Doctor"])
	}
}

func TestVersionUsesInjectedValue(t *testing.T) {
	oldVersion := version
	version = "v9.9.9-test"
	defer func() { version = oldVersion }()

	r := newFakeRunner()
	buf := &bytes.Buffer{}
	cmd := buildRootCmd(r, buf)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}
	if !strings.Contains(buf.String(), "envsync v9.9.9-test") {
		t.Fatalf("unexpected output: %q", buf.String())
	}
}

func TestCommandArgValidation(t *testing.T) {
	r := newFakeRunner()
	buf := &bytes.Buffer{}
	cmd := buildRootCmd(r, buf)
	cmd.SetArgs([]string{"set", "ONLY_KEY"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected args validation error")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthCommandWiring(t *testing.T) {
	r := newFakeRunner()
	buf := &bytes.Buffer{}

	tests := []struct {
		args []string
		call string
	}{
		{[]string{"login"}, "Login"},
		{[]string{"logout"}, "Logout"},
		{[]string{"whoami"}, "WhoAmI"},
	}

	for _, tc := range tests {
		cmd := buildRootCmd(r, buf)
		cmd.SetArgs(tc.args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", tc.args, err)
		}
		if r.calls[tc.call] == 0 {
			t.Fatalf("expected %s to be called", tc.call)
		}
	}
}
