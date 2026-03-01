package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type principal struct {
	Subject string
	UserID  string
	Email   string
	Scopes  map[string]struct{}
	Orgs    []organizationMembership
	Teams   []teamMembership
	All     bool
	Raw     map[string]any
}

type organizationMembership struct {
	OrganizationID string `json:"organization_id"`
	Role           string `json:"role"`
}

type teamMembership struct {
	TeamID string `json:"team_id"`
	Role   string `json:"role"`
}

type authVerifier struct {
	db            *sql.DB
	patPepper     string
	devToken      string
	issuer        string
	audience      string
	skipAudCheck  bool
	oidcVerifier  *oidc.IDTokenVerifier
	oidcAvailable bool
	now           func() time.Time
}

type storeRepo interface {
	Get(ctx context.Context, ownerID, project string) (*remoteStore, error)
	Put(ctx context.Context, ownerID, actorID, project string, next *remoteStore, expectedRevision int) (*remoteStore, error)
}

type cloudServer struct {
	repo          storeRepo
	verifier      *authVerifier
	maxBodyBytes  int64
	projectRegexp *regexp.Regexp
}

type remoteStore struct {
	Version     int            `json:"version"`
	Revision    int            `json:"revision"`
	SaltB64     string         `json:"salt_b64,omitempty"`
	KeyCheckB64 string         `json:"key_check_b64,omitempty"`
	Teams       map[string]any `json:"teams,omitempty"`
	Projects    map[string]any `json:"projects"`
}

type memoryRepo struct {
	mu   sync.Mutex
	data map[string]*remoteStore
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

type contextKey string

const requestIDKey contextKey = "request_id"

var errConflict = errors.New("revision conflict")

const projectNamePattern = `^[a-z0-9][a-z0-9_-]{0,62}$`

func main() {
	addr := strings.TrimSpace(os.Getenv("ENVSYNC_CLOUD_ADDR"))
	if addr == "" {
		if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
			addr = ":" + port
		} else {
			addr = ":8081"
		}
	}
	databaseURL := strings.TrimSpace(os.Getenv("ENVSYNC_CLOUD_DATABASE_URL"))
	useMemory := envBool("ENVSYNC_CLOUD_INMEMORY", false)
	maxBodyBytes := envInt64("ENVSYNC_CLOUD_MAX_BODY_BYTES", 1<<20)
	rateLimitRPM := envInt("ENVSYNC_CLOUD_RATE_LIMIT_RPM", 240)
	rateLimitBurst := envInt("ENVSYNC_CLOUD_RATE_LIMIT_BURST", 40)
	if rateLimitBurst <= 0 {
		rateLimitBurst = 40
	}
	if maxBodyBytes <= 0 {
		maxBodyBytes = 1 << 20
	}
	projectRegexp := regexp.MustCompile(projectNamePattern)

	var db *sql.DB
	var repo storeRepo
	if useMemory {
		repo = &memoryRepo{data: map[string]*remoteStore{}}
		log.Printf("envsync-cloud: using in-memory store")
	} else {
		if databaseURL == "" {
			log.Fatal("ENVSYNC_CLOUD_DATABASE_URL is required unless ENVSYNC_CLOUD_INMEMORY=true")
		}
		openedDB, err := sql.Open("pgx", databaseURL)
		if err != nil {
			log.Fatalf("open db: %v", err)
		}
		db = openedDB
		if err := db.Ping(); err != nil {
			log.Fatalf("ping db: %v", err)
		}
		if err := runMigrations(db); err != nil {
			log.Fatalf("run migrations: %v", err)
		}
		repo = &pgRepo{db: db}
		log.Printf("envsync-cloud: connected to postgres")
	}

	verifier, err := newAuthVerifier(db)
	if err != nil {
		log.Fatalf("init auth verifier: %v", err)
	}

	srv := &cloudServer{repo: repo, verifier: verifier, maxBodyBytes: maxBodyBytes, projectRegexp: projectRegexp}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", srv.handleHealth)
	mux.HandleFunc("/v1/me", srv.handleMe)
	mux.HandleFunc("/v1/store", srv.handleStore)
	mux.HandleFunc("/v1/tokens", srv.handleTokens)
	mux.HandleFunc("/v1/tokens/", srv.handleTokens)

	handler := http.Handler(mux)
	handler = withRequestID(handler)
	handler = withRateLimit(handler, newRateLimiter(rateLimitRPM, rateLimitBurst))
	handler = withRequestLog(handler)

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
	}()

	log.Printf("envsync-cloud listening on %s", addr)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("listen: %v", err)
	}
}

func withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("request_id=%s method=%s path=%s status=%d duration_ms=%d remote=%s",
			requestID(r.Context()),
			r.Method,
			r.URL.Path,
			rec.status,
			time.Since(start).Milliseconds(),
			r.RemoteAddr,
		)
	})
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if requestID == "" {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-Id", requestID)
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *cloudServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *cloudServer) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	p, err := s.verifier.authenticate(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":    p.UserID,
			"email": p.Email,
		},
		"organizations": p.Orgs,
		"teams":         p.Teams,
	})
}

func (s *cloudServer) handleStore(w http.ResponseWriter, r *http.Request) {
	p, err := s.verifier.authenticate(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	ownerID, err := s.resolveOwner(p, r.URL.Query().Get("organization_id"), r.URL.Query().Get("team_id"), r.Method)
	if err != nil {
		if errors.Is(err, errForbidden) {
			writeError(w, r, http.StatusForbidden, "forbidden", err.Error())
			return
		}
		writeError(w, r, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	project, err := s.normalizeProject(r.URL.Query().Get("project"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_project", err.Error())
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !p.hasScope("store:read") {
			writeError(w, r, http.StatusForbidden, "forbidden", "token missing scope store:read")
			return
		}
		store, err := s.repo.Get(r.Context(), ownerID, project)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", "read store failed")
			return
		}
		writeJSON(w, http.StatusOK, store)
	case http.MethodPut:
		if !p.hasScope("store:write") {
			writeError(w, r, http.StatusForbidden, "forbidden", "token missing scope store:write")
			return
		}
		match := strings.TrimSpace(r.Header.Get("If-Match"))
		if match == "" {
			writeError(w, r, http.StatusPreconditionRequired, "precondition_required", "If-Match required")
			return
		}
		expectedRevision, err := strconv.Atoi(match)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "bad_request", "invalid If-Match")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, s.maxBodyBytes)
		defer r.Body.Close()
		var next remoteStore
		if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
			var bodyErr *http.MaxBytesError
			if errors.As(err, &bodyErr) {
				writeError(w, r, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds maximum allowed size")
				return
			}
			writeError(w, r, http.StatusBadRequest, "bad_request", "invalid JSON payload")
			return
		}
		saved, err := s.repo.Put(r.Context(), ownerID, p.UserID, project, &next, expectedRevision)
		if err != nil {
			if errors.Is(err, errConflict) {
				writeError(w, r, http.StatusConflict, "conflict", err.Error())
				return
			}
			writeError(w, r, http.StatusInternalServerError, "internal_error", "write store failed")
			return
		}
		writeJSON(w, http.StatusOK, saved)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (s *cloudServer) handleTokens(w http.ResponseWriter, r *http.Request) {
	p, err := s.verifier.authenticate(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	if !p.hasScope("tokens:write") {
		writeError(w, r, http.StatusForbidden, "forbidden", "token missing scope tokens:write")
		return
	}
	if s.verifier.db == nil {
		writeError(w, r, http.StatusServiceUnavailable, "unavailable", "token management requires postgres mode")
		return
	}

	if r.URL.Path == "/v1/tokens" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		s.createToken(w, r, p)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/v1/tokens/") {
		writeError(w, r, http.StatusNotFound, "not_found", "not found")
		return
	}
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	tokenID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/tokens/"))
	if tokenID == "" {
		writeError(w, r, http.StatusBadRequest, "bad_request", "token id is required")
		return
	}
	if err := s.verifier.revokeToken(r.Context(), p.UserID, tokenID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, r, http.StatusNotFound, "not_found", "token not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", "failed to revoke token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *cloudServer) createToken(w http.ResponseWriter, r *http.Request, p *principal) {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	var req struct {
		Scopes    []string `json:"scopes"`
		ExpiresAt string   `json:"expires_at,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "bad_request", "invalid JSON payload")
		return
	}
	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = []string{"profile:read", "store:read", "store:write"}
	}
	if err := validateScopes(scopes); err != nil {
		writeError(w, r, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	var expiresAt *time.Time
	if strings.TrimSpace(req.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "bad_request", "expires_at must be RFC3339")
			return
		}
		expiresAt = &parsed
	}

	issued, err := s.verifier.issueToken(r.Context(), p, scopes, expiresAt)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "failed to issue token")
		return
	}
	writeJSON(w, http.StatusCreated, issued)
}

func env(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func envInt(name string, fallback int) int {
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

func envInt64(name string, fallback int64) int64 {
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

func envBool(name string, fallback bool) bool {
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

func newAuthVerifier(db *sql.DB) (*authVerifier, error) {
	v := &authVerifier{
		db:           db,
		patPepper:    strings.TrimSpace(os.Getenv("ENVSYNC_CLOUD_PAT_PEPPER")),
		devToken:     strings.TrimSpace(os.Getenv("ENVSYNC_CLOUD_DEV_TOKEN")),
		issuer:       strings.TrimSpace(os.Getenv("ENVSYNC_CLOUD_JWT_ISSUER")),
		audience:     strings.TrimSpace(os.Getenv("ENVSYNC_CLOUD_JWT_AUDIENCE")),
		skipAudCheck: envBool("ENVSYNC_CLOUD_JWT_SKIP_AUD_CHECK", false),
		now:          time.Now,
	}
	if v.db != nil && v.patPepper == "" {
		log.Printf("warning: ENVSYNC_CLOUD_PAT_PEPPER is empty; PAT authentication is disabled")
	}
	if v.issuer == "" {
		return v, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, v.issuer)
	if err != nil {
		return nil, fmt.Errorf("init oidc provider: %w", err)
	}
	verifierCfg := &oidc.Config{
		ClientID:          v.audience,
		SkipClientIDCheck: v.audience == "" || v.skipAudCheck,
	}
	v.oidcVerifier = provider.Verifier(verifierCfg)
	v.oidcAvailable = true
	return v, nil
}

func (v *authVerifier) authenticate(r *http.Request) (*principal, error) {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" || !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return nil, errors.New("missing bearer token")
	}
	token := strings.TrimSpace(auth[len("Bearer "):])
	if token == "" {
		return nil, errors.New("missing bearer token")
	}
	if pat, err := v.authenticatePAT(r.Context(), token); err == nil && pat != nil {
		return pat, nil
	} else if err != nil {
		return nil, err
	}

	if v.devToken != "" && token == v.devToken {
		return &principal{
			Subject: "dev-user",
			UserID:  "dev-user",
			Email:   "dev@example.com",
			All:     true,
			Scopes:  map[string]struct{}{},
			Raw:     map[string]any{"sub": "dev-user", "email": "dev@example.com"},
		}, nil
	}
	if !v.oidcAvailable {
		return nil, errors.New("token verification is not configured")
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	idToken, err := v.oidcVerifier.Verify(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("decode token claims: %w", err)
	}
	sub := stringClaim(claims, "sub")
	if sub == "" {
		return nil, errors.New("token missing sub claim")
	}
	return &principal{
		Subject: sub,
		UserID:  sub,
		Email:   stringClaim(claims, "email"),
		All:     true,
		Scopes:  map[string]struct{}{},
		Raw:     claims,
	}, nil
}

func (v *authVerifier) authenticatePAT(ctx context.Context, rawToken string) (*principal, error) {
	if v.db == nil || strings.TrimSpace(v.patPepper) == "" {
		return nil, nil
	}
	prefix := extractTokenPrefix(rawToken)
	if prefix == "" {
		return nil, nil
	}
	rows, err := v.db.QueryContext(ctx, `
SELECT pat.id::text, pat.user_id::text, pat.token_hash, pat.expires_at, pat.revoked_at,
       COALESCE(to_json(pat.scopes)::text, '[]') AS scopes_json,
       COALESCE(u.email, '')
FROM personal_access_tokens pat
JOIN users u ON u.id = pat.user_id
WHERE pat.token_prefix = $1
ORDER BY pat.created_at DESC
`, prefix)
	if err != nil {
		return nil, fmt.Errorf("verify token: %w", err)
	}
	defer rows.Close()

	hash := v.hashToken(rawToken)
	for rows.Next() {
		var (
			tokenID    string
			userID     string
			storedHash string
			expiresAt  sql.NullTime
			revokedAt  sql.NullTime
			scopesJSON string
			email      string
		)
		if err := rows.Scan(&tokenID, &userID, &storedHash, &expiresAt, &revokedAt, &scopesJSON, &email); err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}
		if subtle.ConstantTimeCompare([]byte(storedHash), []byte(hash)) != 1 {
			continue
		}
		if revokedAt.Valid {
			return nil, errors.New("token is revoked")
		}
		if expiresAt.Valid && v.now().After(expiresAt.Time) {
			return nil, errors.New("token is expired")
		}
		var scopes []string
		if err := json.Unmarshal([]byte(scopesJSON), &scopes); err != nil {
			return nil, fmt.Errorf("decode token scopes: %w", err)
		}
		if _, err := v.db.ExecContext(ctx, `UPDATE personal_access_tokens SET last_used_at = NOW() WHERE id = $1::uuid`, tokenID); err != nil {
			log.Printf("warning: failed to update token last_used_at token_id=%s err=%v", tokenID, err)
		}
		orgs, err := v.loadMemberships(ctx, userID)
		if err != nil {
			return nil, err
		}
		teams, err := v.loadTeamMemberships(ctx, userID)
		if err != nil {
			return nil, err
		}
		return &principal{
			Subject: userID,
			UserID:  userID,
			Email:   email,
			Scopes:  toScopeSet(scopes),
			Orgs:    orgs,
			Teams:   teams,
			Raw: map[string]any{
				"token_id": tokenID,
			},
		}, nil
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tokens: %w", err)
	}
	return nil, nil
}

func (v *authVerifier) loadMemberships(ctx context.Context, userID string) ([]organizationMembership, error) {
	if v.db == nil {
		return nil, nil
	}
	rows, err := v.db.QueryContext(ctx, `
SELECT organization_id::text, role
FROM organization_members
WHERE user_id = $1::uuid
ORDER BY organization_id
`, userID)
	if err != nil {
		return nil, fmt.Errorf("load memberships: %w", err)
	}
	defer rows.Close()
	out := make([]organizationMembership, 0)
	for rows.Next() {
		var m organizationMembership
		if err := rows.Scan(&m.OrganizationID, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (v *authVerifier) loadTeamMemberships(ctx context.Context, userID string) ([]teamMembership, error) {
	if v.db == nil {
		return nil, nil
	}
	rows, err := v.db.QueryContext(ctx, `
SELECT team_id::text, role
FROM team_members
WHERE user_id = $1::uuid
ORDER BY team_id
`, userID)
	if err != nil {
		return nil, fmt.Errorf("load team memberships: %w", err)
	}
	defer rows.Close()
	out := make([]teamMembership, 0)
	for rows.Next() {
		var m teamMembership
		if err := rows.Scan(&m.TeamID, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (v *authVerifier) hashToken(token string) string {
	mac := hmac.New(sha256.New, []byte(v.patPepper))
	_, _ = mac.Write([]byte(token))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func (v *authVerifier) ensureUserID(ctx context.Context, p *principal) (string, error) {
	if v.db == nil {
		return p.UserID, nil
	}
	if isUUID(p.UserID) {
		_, err := v.db.ExecContext(ctx, `
INSERT INTO users (id, email)
VALUES ($1::uuid, NULLIF($2, ''))
ON CONFLICT (id) DO UPDATE SET updated_at = NOW(), email = COALESCE(NULLIF(EXCLUDED.email, ''), users.email)
`, p.UserID, p.Email)
		if err != nil {
			return "", err
		}
		return p.UserID, nil
	}
	var id string
	err := v.db.QueryRowContext(ctx, `SELECT id::text FROM users WHERE workos_user_id = $1`, p.Subject).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	err = v.db.QueryRowContext(ctx, `
INSERT INTO users (workos_user_id, email)
VALUES ($1, NULLIF($2, ''))
RETURNING id::text
`, p.Subject, p.Email).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (v *authVerifier) issueToken(ctx context.Context, p *principal, scopes []string, expiresAt *time.Time) (map[string]any, error) {
	if v.db == nil {
		return nil, errors.New("db is not configured")
	}
	if strings.TrimSpace(v.patPepper) == "" {
		return nil, errors.New("ENVSYNC_CLOUD_PAT_PEPPER is required")
	}
	userID, err := v.ensureUserID(ctx, p)
	if err != nil {
		return nil, err
	}
	rawToken, prefix, err := generatePAT()
	if err != nil {
		return nil, err
	}
	tokenHash := v.hashToken(rawToken)
	var tokenID string
	var expires any
	if expiresAt != nil {
		expires = *expiresAt
	}
	err = v.db.QueryRowContext(ctx, `
INSERT INTO personal_access_tokens (user_id, token_prefix, token_hash, scopes, expires_at)
VALUES ($1::uuid, $2, $3, $4::text[], $5)
RETURNING id::text
`, userID, prefix, tokenHash, scopes, expires).Scan(&tokenID)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"id":           tokenID,
		"token":        rawToken,
		"token_prefix": prefix,
		"scopes":       scopes,
	}
	if expiresAt != nil {
		out["expires_at"] = expiresAt.UTC().Format(time.RFC3339)
	}
	return out, nil
}

func (v *authVerifier) revokeToken(ctx context.Context, userID, tokenID string) error {
	if v.db == nil {
		return errors.New("db is not configured")
	}
	res, err := v.db.ExecContext(ctx, `
UPDATE personal_access_tokens
SET revoked_at = NOW()
WHERE id = $1::uuid AND user_id = $2::uuid AND revoked_at IS NULL
`, tokenID, userID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func stringClaim(claims map[string]any, key string) string {
	if claims == nil {
		return ""
	}
	if v, ok := claims[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func runMigrations(db *sql.DB) error {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	for _, name := range files {
		raw, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(raw)); err != nil {
			return fmt.Errorf("migration %s failed: %w", name, err)
		}
	}
	return nil
}

type pgRepo struct {
	db *sql.DB
}

func (r *pgRepo) Get(ctx context.Context, ownerID, project string) (*remoteStore, error) {
	loadRow := func(candidateOwner string) *sql.Row {
		return r.db.QueryRowContext(ctx, `
SELECT revision, payload_json, salt_b64, key_check_b64
FROM vault_snapshots
WHERE owner_user_id = $1 AND project_name = $2
`, candidateOwner, project)
	}
	row := loadRow(ownerID)

	var (
		revision   int
		payloadRaw []byte
		saltB64    sql.NullString
		keyCheck   sql.NullString
	)
	if err := row.Scan(&revision, &payloadRaw, &saltB64, &keyCheck); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if legacyOwner := legacyOwnerID(ownerID); legacyOwner != "" {
				row = loadRow(legacyOwner)
				err = row.Scan(&revision, &payloadRaw, &saltB64, &keyCheck)
			}
		}
		if errors.Is(err, sql.ErrNoRows) {
			return &remoteStore{
				Version:  1,
				Revision: 0,
				Projects: map[string]any{},
				Teams:    map[string]any{},
			}, nil
		}
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		return nil, err
	}
	projects, _ := payload["projects"].(map[string]any)
	teams, _ := payload["teams"].(map[string]any)
	if projects == nil {
		projects = map[string]any{}
	}
	if teams == nil {
		teams = map[string]any{}
	}
	return &remoteStore{
		Version:     1,
		Revision:    revision,
		Projects:    projects,
		Teams:       teams,
		SaltB64:     saltB64.String,
		KeyCheckB64: keyCheck.String,
	}, nil
}

func legacyOwnerID(ownerID string) string {
	if strings.HasPrefix(ownerID, "org:") {
		return strings.TrimSpace(strings.TrimPrefix(ownerID, "org:"))
	}
	if strings.HasPrefix(ownerID, "team:") {
		return strings.TrimSpace(strings.TrimPrefix(ownerID, "team:"))
	}
	return ""
}

func (r *pgRepo) Put(ctx context.Context, ownerID, actorID, project string, next *remoteStore, expectedRevision int) (*remoteStore, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var currentRevision int
	row := tx.QueryRowContext(ctx, `
SELECT revision
FROM vault_snapshots
WHERE owner_user_id = $1 AND project_name = $2
FOR UPDATE
`, ownerID, project)
	if err := row.Scan(&currentRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			currentRevision = 0
		} else {
			return nil, err
		}
	}
	if currentRevision != expectedRevision {
		return nil, fmt.Errorf("%w: expected %d, got %d", errConflict, expectedRevision, currentRevision)
	}
	nextRevision := currentRevision + 1

	payload := map[string]any{
		"projects": next.Projects,
		"teams":    next.Teams,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO vault_snapshots (
  owner_user_id, project_name, revision, payload_json, salt_b64, key_check_b64, updated_by_user_id, updated_at
)
VALUES ($1,$2,$3,$4::jsonb,$5,$6,$7,NOW())
ON CONFLICT (owner_user_id, project_name)
DO UPDATE SET
  revision = EXCLUDED.revision,
  payload_json = EXCLUDED.payload_json,
  salt_b64 = EXCLUDED.salt_b64,
  key_check_b64 = EXCLUDED.key_check_b64,
  updated_by_user_id = EXCLUDED.updated_by_user_id,
  updated_at = NOW()
`, ownerID, project, nextRevision, payloadJSON, nullIfEmpty(next.SaltB64), nullIfEmpty(next.KeyCheckB64), actorID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO audit_events (actor_user_id, action, vault_owner_user_id, project_name, metadata_json)
VALUES ($1, 'store_put', $2, $3, $4::jsonb)
`, actorID, ownerID, project, []byte(`{"source":"envsync-cloud"}`)); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	out := *next
	out.Revision = nextRevision
	if out.Projects == nil {
		out.Projects = map[string]any{}
	}
	if out.Teams == nil {
		out.Teams = map[string]any{}
	}
	return &out, nil
}

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

var errForbidden = errors.New("forbidden")

func (s *cloudServer) resolveOwner(p *principal, orgIDRaw, teamIDRaw, method string) (string, error) {
	if p == nil {
		return "", errors.New("missing principal")
	}
	orgID := strings.TrimSpace(orgIDRaw)
	teamID := strings.TrimSpace(teamIDRaw)
	if orgID != "" && teamID != "" {
		return "", errors.New("organization_id and team_id are mutually exclusive")
	}
	if orgID == "" && teamID == "" {
		return p.UserID, nil
	}
	if p.All {
		if teamID != "" {
			return "team:" + teamID, nil
		}
		return "org:" + orgID, nil
	}
	required := "reader"
	if method == http.MethodPut {
		required = "maintainer"
	}
	if teamID != "" {
		for _, membership := range p.Teams {
			if strings.EqualFold(strings.TrimSpace(membership.TeamID), teamID) && roleAllows(membership.Role, required) {
				return "team:" + teamID, nil
			}
		}
		return "", fmt.Errorf("%w: team access denied", errForbidden)
	}
	for _, membership := range p.Orgs {
		if strings.EqualFold(strings.TrimSpace(membership.OrganizationID), orgID) && roleAllows(membership.Role, required) {
			return "org:" + orgID, nil
		}
	}
	return "", fmt.Errorf("%w: organization access denied", errForbidden)
}

func roleAllows(role, required string) bool {
	rank := map[string]int{"reader": 1, "maintainer": 2, "admin": 3}
	current := rank[strings.ToLower(strings.TrimSpace(role))]
	need := rank[strings.ToLower(strings.TrimSpace(required))]
	return current >= need && need > 0
}

func (p *principal) hasScope(scope string) bool {
	if p == nil {
		return false
	}
	if p.All {
		return true
	}
	if _, ok := p.Scopes["*"]; ok {
		return true
	}
	_, ok := p.Scopes[scope]
	return ok
}

func (s *cloudServer) normalizeProject(raw string) (string, error) {
	project := strings.ToLower(strings.TrimSpace(raw))
	if project == "" {
		project = "default"
	}
	if !s.projectRegexp.MatchString(project) {
		return "", fmt.Errorf("project must match %s", projectNamePattern)
	}
	return project, nil
}

func requestID(ctx context.Context) string {
	v := ctx.Value(requestIDKey)
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error":      code,
		"message":    message,
		"request_id": requestID(r.Context()),
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func newRequestID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("req-%x", b)
}

func extractTokenPrefix(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if idx := strings.Index(token, "."); idx > 0 {
		return token[:idx]
	}
	if len(token) < 12 {
		return token
	}
	return token[:12]
}

func toScopeSet(scopes []string) map[string]struct{} {
	out := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		s := strings.TrimSpace(scope)
		if s == "" {
			continue
		}
		out[s] = struct{}{}
	}
	return out
}

func validateScopes(scopes []string) error {
	allowed := map[string]struct{}{
		"profile:read": {},
		"store:read":   {},
		"store:write":  {},
		"tokens:write": {},
		"*":            {},
	}
	for _, scope := range scopes {
		s := strings.TrimSpace(scope)
		if s == "" {
			return errors.New("scope entries cannot be empty")
		}
		if _, ok := allowed[s]; !ok {
			return fmt.Errorf("unsupported scope %q", s)
		}
	}
	return nil
}

func generatePAT() (raw, prefix string, err error) {
	prefixRand := make([]byte, 6)
	secretRand := make([]byte, 18)
	if _, err := rand.Read(prefixRand); err != nil {
		return "", "", err
	}
	if _, err := rand.Read(secretRand); err != nil {
		return "", "", err
	}
	prefix = fmt.Sprintf("espat_%x", prefixRand)
	raw = fmt.Sprintf("%s.%x", prefix, secretRand)
	return raw, prefix, nil
}

func isUUID(v string) bool {
	v = strings.TrimSpace(v)
	if len(v) != 36 {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
				continue
			}
			return false
		}
	}
	return true
}

type rateLimiter struct {
	mu      sync.Mutex
	rpm     int
	burst   int
	clients map[string]*rateState
}

type rateState struct {
	windowStart time.Time
	count       int
}

func newRateLimiter(rpm, burst int) *rateLimiter {
	return &rateLimiter{
		rpm:     rpm,
		burst:   burst,
		clients: map[string]*rateState{},
	}
}

func (l *rateLimiter) allow(ip string) bool {
	if l == nil || l.rpm <= 0 {
		return true
	}
	now := time.Now().UTC()
	l.mu.Lock()
	defer l.mu.Unlock()
	state := l.clients[ip]
	if state == nil {
		l.clients[ip] = &rateState{windowStart: now, count: 1}
		return true
	}
	if now.Sub(state.windowStart) >= time.Minute {
		state.windowStart = now
		state.count = 1
		return true
	}
	limit := l.rpm + l.burst
	if state.count >= limit {
		return false
	}
	state.count++
	return true
}

func withRateLimit(next http.Handler, limiter *rateLimiter) http.Handler {
	if limiter == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		ip := clientIP(r)
		if !limiter.allow(ip) {
			writeError(w, r, http.StatusTooManyRequests, "too_many_requests", "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (m *memoryRepo) Get(_ context.Context, ownerID, project string) (*remoteStore, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := ownerID + ":" + project
	if existing := m.data[key]; existing != nil {
		out := *existing
		return &out, nil
	}
	return &remoteStore{Version: 1, Revision: 0, Projects: map[string]any{}, Teams: map[string]any{}}, nil
}

func (m *memoryRepo) Put(_ context.Context, ownerID, actorID string, project string, next *remoteStore, expectedRevision int) (*remoteStore, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = actorID // currently unused; required by interface
	key := ownerID + ":" + project
	current := 0
	if existing := m.data[key]; existing != nil {
		current = existing.Revision
	}
	if current != expectedRevision {
		return nil, fmt.Errorf("%w: expected %d, got %d", errConflict, expectedRevision, current)
	}
	out := *next
	out.Revision = current + 1
	if out.Projects == nil {
		out.Projects = map[string]any{}
	}
	if out.Teams == nil {
		out.Teams = map[string]any{}
	}
	m.data[key] = &out
	return &out, nil
}
