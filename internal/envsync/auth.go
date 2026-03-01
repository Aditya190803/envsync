package envsync

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type cloudSession struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	Email        string `json:"email,omitempty"`
}

func (a *App) cloudBaseURL() string {
	if v := strings.TrimSuffix(strings.TrimSpace(a.CloudURL), "/"); v != "" {
		return v
	}
	return ""
}

func (a *App) Login() error {
	baseURL := a.cloudBaseURL()
	if baseURL == "" {
		return errors.New("cloud URL is not configured; set ENVSYNC_CLOUD_URL")
	}

	token := strings.TrimSpace(os.Getenv("ENVSYNC_CLOUD_ACCESS_TOKEN"))
	if token == "" {
		fmt.Fprint(a.Stderr, "Cloud access token: ")
		reader := bufio.NewReader(a.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		token = strings.TrimSpace(line)
	}
	if token == "" {
		return errors.New("access token cannot be empty")
	}

	me, err := a.cloudMe(token)
	if err != nil {
		return err
	}

	session := &cloudSession{
		AccessToken: token,
		UserID:      getString(me, "id"),
		Email:       getString(me, "email"),
	}
	if err := a.saveCloudSession(session); err != nil {
		return err
	}

	if session.Email != "" {
		fmt.Fprintf(a.Stdout, "%s %s\n", cSuccess("logged in as"), cBold(session.Email))
	} else {
		fmt.Fprintln(a.Stdout, cSuccess("login successful"))
	}
	a.logAudit("login", nil, map[string]any{"email": session.Email, "user_id": session.UserID})
	return nil
}

func (a *App) Logout() error {
	_ = os.Remove(a.SessionPath)
	_ = a.clearSessionKeychain()
	fmt.Fprintln(a.Stdout, cSuccess("logged out"))
	a.logAudit("logout", nil, nil)
	return nil
}

func (a *App) WhoAmI() error {
	session, err := a.loadCloudSession()
	if err != nil {
		return errors.New("not logged in; run `envsync login`")
	}
	me, err := a.cloudMe(session.AccessToken)
	if err != nil {
		return err
	}

	email := getString(me, "email")
	if email == "" {
		email = session.Email
	}
	id := getString(me, "id")
	if id == "" {
		id = session.UserID
	}
	if email != "" || id != "" {
		if email != "" {
			fmt.Fprintf(a.Stdout, "email: %s\n", email)
		}
		if id != "" {
			fmt.Fprintf(a.Stdout, "id: %s\n", id)
		}
		return nil
	}
	raw, err := json.MarshalIndent(me, "", "  ")
	if err != nil {
		return err
	}
	_, _ = a.Stdout.Write(raw)
	_, _ = a.Stdout.Write([]byte("\n"))
	return nil
}

func (a *App) cloudAccessToken() (string, error) {
	session, err := a.loadCloudSession()
	if err != nil {
		return "", errors.New("cloud login required; run `envsync login`")
	}
	if strings.TrimSpace(session.ExpiresAt) != "" {
		nowFn := a.Now
		if nowFn == nil {
			nowFn = time.Now
		}
		if expiry, err := time.Parse(time.RFC3339, session.ExpiresAt); err == nil && nowFn().After(expiry) {
			return "", errors.New("cloud session expired; run `envsync login`")
		}
	}
	if strings.TrimSpace(session.AccessToken) == "" {
		return "", errors.New("cloud session is invalid; run `envsync login`")
	}
	return session.AccessToken, nil
}

func (a *App) loadCloudSession() (*cloudSession, error) {
	if fromKC, err := a.sessionFromKeychain(); err == nil && strings.TrimSpace(fromKC) != "" {
		var s cloudSession
		if err := json.Unmarshal([]byte(fromKC), &s); err == nil {
			return &s, nil
		}
	}
	b, err := os.ReadFile(a.SessionPath)
	if err != nil {
		return nil, err
	}
	var s cloudSession
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (a *App) saveCloudSession(session *cloudSession) error {
	b, err := json.Marshal(session)
	if err != nil {
		return err
	}
	if err := a.storeSessionKeychain(string(b)); err == nil {
		_ = os.Remove(a.SessionPath)
		return nil
	}
	if err := os.MkdirAll(a.ConfigDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(a.SessionPath, b, 0o600)
}

func (a *App) hasCloudSession() bool {
	_, err := a.loadCloudSession()
	return err == nil
}

func (a *App) cloudMe(token string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, a.cloudBaseURL()+"/v1/me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("cloud identity check failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if user, ok := payload["user"].(map[string]any); ok {
		return user, nil
	}
	return payload, nil
}

func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func (a *App) authHeaderToken() string {
	if tok := strings.TrimSpace(a.RemoteToken); tok != "" {
		return tok
	}
	token, err := a.cloudAccessToken()
	if err != nil {
		return ""
	}
	return token
}

func addAuthHeader(req *http.Request, token string) {
	if strings.TrimSpace(token) == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
}

func decodeRemoteStoreBody(r io.Reader) (*RemoteStore, error) {
	var decoded RemoteStore
	if err := json.NewDecoder(r).Decode(&decoded); err != nil {
		return nil, err
	}
	if decoded.Projects == nil {
		decoded.Projects = map[string]*Project{}
	}
	if decoded.Teams == nil {
		decoded.Teams = map[string]*Team{}
	}
	return &decoded, nil
}

func encodeRemoteStoreBody(remote *RemoteStore) ([]byte, error) {
	buf := bytes.Buffer{}
	if err := json.NewEncoder(&buf).Encode(remote); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
