package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func newTestCloudServer() *cloudServer {
	return &cloudServer{
		repo: &memoryRepo{data: map[string]*remoteStore{}},
		verifier: &authVerifier{
			devToken: "test-token",
		},
		maxBodyBytes:  1 << 20,
		projectRegexp: mustProjectRegexp(),
	}
}

func mustProjectRegexp() *regexp.Regexp {
	return regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
}

func TestMeRequiresAuth(t *testing.T) {
	s := newTestCloudServer()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	rec := httptest.NewRecorder()
	s.handleMe(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMeReturnsPrincipal(t *testing.T) {
	s := newTestCloudServer()
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
	user, _ := payload["user"].(map[string]any)
	if user["id"] != "dev-user" {
		t.Fatalf("expected dev-user id, got %v", user["id"])
	}
}

func TestStorePutAndGet(t *testing.T) {
	s := newTestCloudServer()
	body := map[string]any{
		"version":       1,
		"revision":      0,
		"salt_b64":      "abc",
		"key_check_b64": "xyz",
		"projects": map[string]any{
			"api": map[string]any{"name": "api"},
		},
		"teams": map[string]any{},
	}
	raw, _ := json.Marshal(body)
	putReq := httptest.NewRequest(http.MethodPut, "/v1/store?project=api", bytes.NewReader(raw))
	putReq.Header.Set("Authorization", "Bearer test-token")
	putReq.Header.Set("If-Match", "0")
	putRec := httptest.NewRecorder()
	s.handleStore(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("put expected 200, got %d body=%s", putRec.Code, putRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/store?project=api", nil)
	getReq.Header.Set("Authorization", "Bearer test-token")
	getRec := httptest.NewRecorder()
	s.handleStore(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get expected 200, got %d", getRec.Code)
	}
	var store remoteStore
	if err := json.Unmarshal(getRec.Body.Bytes(), &store); err != nil {
		t.Fatalf("decode store: %v", err)
	}
	if store.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", store.Revision)
	}
}

func TestStoreRevisionConflict(t *testing.T) {
	s := newTestCloudServer()
	payload := []byte(`{"version":1,"revision":0,"projects":{},"teams":{}}`)

	req1 := httptest.NewRequest(http.MethodPut, "/v1/store?project=api", bytes.NewReader(payload))
	req1.Header.Set("Authorization", "Bearer test-token")
	req1.Header.Set("If-Match", "0")
	rec1 := httptest.NewRecorder()
	s.handleStore(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first put expected 200, got %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPut, "/v1/store?project=api", bytes.NewReader(payload))
	req2.Header.Set("Authorization", "Bearer test-token")
	req2.Header.Set("If-Match", "0")
	rec2 := httptest.NewRecorder()
	s.handleStore(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second put expected 409, got %d", rec2.Code)
	}
}

func TestStoreProjectNormalization(t *testing.T) {
	s := newTestCloudServer()
	payload := []byte(`{"version":1,"revision":0,"projects":{},"teams":{}}`)

	putReq := httptest.NewRequest(http.MethodPut, "/v1/store?project=My_App", bytes.NewReader(payload))
	putReq.Header.Set("Authorization", "Bearer test-token")
	putReq.Header.Set("If-Match", "0")
	putRec := httptest.NewRecorder()
	s.handleStore(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("put expected 200, got %d body=%s", putRec.Code, putRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/store?project=my_app", nil)
	getReq.Header.Set("Authorization", "Bearer test-token")
	getRec := httptest.NewRecorder()
	s.handleStore(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get expected 200, got %d", getRec.Code)
	}
}

func TestStoreRejectsInvalidProject(t *testing.T) {
	s := newTestCloudServer()
	getReq := httptest.NewRequest(http.MethodGet, "/v1/store?project=Bad/Name", nil)
	getReq.Header.Set("Authorization", "Bearer test-token")
	getRec := httptest.NewRecorder()
	s.handleStore(getRec, getReq)
	if getRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", getRec.Code)
	}
	if !strings.Contains(getRec.Body.String(), "invalid_project") {
		t.Fatalf("expected invalid_project code, got %s", getRec.Body.String())
	}
}

func TestUnauthorizedIncludesRequestID(t *testing.T) {
	s := newTestCloudServer()
	h := withRequestID(http.HandlerFunc(s.handleMe))
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("X-Request-Id", "req-test")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Request-Id"); got != "req-test" {
		t.Fatalf("expected request id header req-test, got %q", got)
	}
	if !strings.Contains(rec.Body.String(), "req-test") {
		t.Fatalf("expected request id in body, got %s", rec.Body.String())
	}
}

func TestRateLimiterMiddleware(t *testing.T) {
	limiter := newRateLimiter(1, 0)
	var calls int
	h := withRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}), limiter)

	req1 := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req1.RemoteAddr = "127.0.0.1:1234"
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request expected 200, got %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req2.RemoteAddr = "127.0.0.1:1234"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request expected 429, got %d", rec2.Code)
	}
	if calls != 1 {
		t.Fatalf("expected handler to run once, got %d", calls)
	}
}

func TestResolveOwnerForOrganizationAndTeam(t *testing.T) {
	s := newTestCloudServer()
	p := &principal{
		UserID: "user-1",
		Orgs: []organizationMembership{
			{OrganizationID: "org-1", Role: "maintainer"},
		},
		Teams: []teamMembership{
			{TeamID: "team-1", Role: "reader"},
		},
	}

	owner, err := s.resolveOwner(p, "org-1", "", http.MethodPut)
	if err != nil {
		t.Fatalf("resolve org owner failed: %v", err)
	}
	if owner != "org:org-1" {
		t.Fatalf("expected org owner key, got %q", owner)
	}

	owner, err = s.resolveOwner(p, "", "team-1", http.MethodGet)
	if err != nil {
		t.Fatalf("resolve team owner failed: %v", err)
	}
	if owner != "team:team-1" {
		t.Fatalf("expected team owner key, got %q", owner)
	}
}

func TestResolveOwnerRejectsInsufficientTeamRole(t *testing.T) {
	s := newTestCloudServer()
	p := &principal{
		UserID: "user-1",
		Teams: []teamMembership{
			{TeamID: "team-1", Role: "reader"},
		},
	}

	_, err := s.resolveOwner(p, "", "team-1", http.MethodPut)
	if err == nil {
		t.Fatal("expected forbidden error for team write with reader role")
	}
}

func TestResolveOwnerRejectsOrgAndTeamTogether(t *testing.T) {
	s := newTestCloudServer()
	p := &principal{UserID: "user-1"}

	_, err := s.resolveOwner(p, "org-1", "team-1", http.MethodGet)
	if err == nil {
		t.Fatal("expected error for mutually exclusive organization_id and team_id")
	}
}
