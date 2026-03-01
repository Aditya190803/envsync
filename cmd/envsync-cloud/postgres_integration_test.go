package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
)

func TestPostgresPATTeamStoreAuthorization(t *testing.T) {
	databaseURL := os.Getenv("ENVSYNC_CLOUD_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ENVSYNC_CLOUD_DATABASE_URL to run postgres integration tests")
	}
	pepper := os.Getenv("ENVSYNC_CLOUD_PAT_PEPPER")
	if pepper == "" {
		t.Skip("set ENVSYNC_CLOUD_PAT_PEPPER to run postgres integration tests")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	if err := runMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	if _, err := db.Exec(`
TRUNCATE team_members, teams, personal_access_tokens, organization_members, organizations, vault_snapshots, vaults, users, audit_events CASCADE
`); err != nil {
		t.Fatalf("truncate test tables: %v", err)
	}

	verifier, err := newAuthVerifier(db)
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	server := &cloudServer{
		repo:          &pgRepo{db: db},
		verifier:      verifier,
		maxBodyBytes:  1 << 20,
		projectRegexp: regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`),
	}

	ownerUserID := mustInsertUser(t, db, "owner@example.com")
	memberUserID := mustInsertUser(t, db, "member@example.com")
	teamID := mustInsertTeam(t, db, "platform")
	mustInsertTeamMember(t, db, teamID, ownerUserID, "maintainer")
	mustInsertTeamMember(t, db, teamID, memberUserID, "reader")

	ownerToken := mustIssueToken(t, verifier, &principal{Subject: ownerUserID, UserID: ownerUserID, Email: "owner@example.com"}, []string{"store:read", "store:write"})
	memberToken := mustIssueToken(t, verifier, &principal{Subject: memberUserID, UserID: memberUserID, Email: "member@example.com"}, []string{"store:read", "store:write"})

	payload := []byte(`{"version":1,"revision":0,"projects":{},"teams":{}}`)
	putOwner := httptest.NewRequest(http.MethodPut, "/v1/store?team_id="+teamID+"&project=api", bytes.NewReader(payload))
	putOwner.Header.Set("Authorization", "Bearer "+ownerToken)
	putOwner.Header.Set("If-Match", "0")
	putOwnerRec := httptest.NewRecorder()
	server.handleStore(putOwnerRec, putOwner)
	if putOwnerRec.Code != http.StatusOK {
		t.Fatalf("owner put expected 200, got %d body=%s", putOwnerRec.Code, putOwnerRec.Body.String())
	}

	getMember := httptest.NewRequest(http.MethodGet, "/v1/store?team_id="+teamID+"&project=api", nil)
	getMember.Header.Set("Authorization", "Bearer "+memberToken)
	getMemberRec := httptest.NewRecorder()
	server.handleStore(getMemberRec, getMember)
	if getMemberRec.Code != http.StatusOK {
		t.Fatalf("member get expected 200, got %d body=%s", getMemberRec.Code, getMemberRec.Body.String())
	}

	putMember := httptest.NewRequest(http.MethodPut, "/v1/store?team_id="+teamID+"&project=api", bytes.NewReader([]byte(`{"version":1,"revision":1,"projects":{},"teams":{}}`)))
	putMember.Header.Set("Authorization", "Bearer "+memberToken)
	putMember.Header.Set("If-Match", "1")
	putMemberRec := httptest.NewRecorder()
	server.handleStore(putMemberRec, putMember)
	if putMemberRec.Code != http.StatusForbidden {
		t.Fatalf("member put expected 403, got %d body=%s", putMemberRec.Code, putMemberRec.Body.String())
	}
}

func mustInsertUser(t *testing.T, db *sql.DB, email string) string {
	t.Helper()
	var userID string
	if err := db.QueryRow(`INSERT INTO users (email) VALUES ($1) RETURNING id::text`, email).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return userID
}

func mustInsertTeam(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var teamID string
	if err := db.QueryRow(`INSERT INTO teams (name) VALUES ($1) RETURNING id::text`, name).Scan(&teamID); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	return teamID
}

func mustInsertTeamMember(t *testing.T, db *sql.DB, teamID, userID, role string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO team_members (team_id, user_id, role) VALUES ($1::uuid, $2::uuid, $3)`, teamID, userID, role); err != nil {
		t.Fatalf("insert team member: %v", err)
	}
}

func mustIssueToken(t *testing.T, verifier *authVerifier, p *principal, scopes []string) string {
	t.Helper()
	issued, err := verifier.issueToken(context.Background(), p, scopes, nil)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	raw, ok := issued["token"].(string)
	if !ok || raw == "" {
		t.Fatalf("issued token missing raw token")
	}
	return raw
}

func TestMeIncludesTeams(t *testing.T) {
	s := newTestCloudServer()
	s.verifier.devToken = "test-token"
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleMe(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := payload["teams"]; !ok {
		t.Fatalf("expected teams field in /v1/me response")
	}
}
