package envsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func (a *App) loadRemoteStore() (*RemoteStore, error) {
	switch a.effectiveRemoteMode() {
	case "cloud":
		return a.loadRemoteCloud()
	case "http":
		return a.loadRemoteHTTP()
	default:
		return a.loadRemoteFile()
	}
}

func (a *App) saveRemoteStore(remote *RemoteStore, expectedRevision int) error {
	switch a.effectiveRemoteMode() {
	case "cloud":
		return a.saveRemoteCloud(remote, expectedRevision)
	case "http":
		return a.saveRemoteHTTP(remote, expectedRevision)
	default:
		return a.saveRemoteFile(remote, expectedRevision)
	}
}

func (a *App) effectiveRemoteMode() string {
	mode := strings.ToLower(strings.TrimSpace(a.RemoteMode))
	switch mode {
	case "cloud", "file", "http":
		return mode
	}
	if strings.TrimSpace(a.RemoteURL) != "" {
		return "http"
	}
	if a.cloudBaseURL() != "" && a.hasCloudSession() {
		return "cloud"
	}
	return "file"
}

func (a *App) loadRemoteFile() (*RemoteStore, error) {
	b, err := os.ReadFile(a.RemotePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &RemoteStore{Version: 1, Revision: 0, Teams: map[string]*Team{}, Projects: map[string]*Project{}}, nil
		}
		return nil, err
	}
	var remote RemoteStore
	if err := json.Unmarshal(b, &remote); err != nil {
		return nil, err
	}
	if remote.Projects == nil {
		remote.Projects = map[string]*Project{}
	}
	if remote.Teams == nil {
		remote.Teams = map[string]*Team{}
	}
	return &remote, nil
}

func (a *App) saveRemoteFile(remote *RemoteStore, expectedRevision int) error {
	lockPath := a.RemotePath + ".lock"
	return withExclusiveFileLock(lockPath, func() error {
		current, err := a.loadRemoteFile()
		if err != nil {
			return err
		}
		if current.Revision != expectedRevision {
			return fmt.Errorf("remote changed concurrently: expected revision %d, got %d", expectedRevision, current.Revision)
		}
		remote.Revision = current.Revision + 1
		if err := os.MkdirAll(filepath.Dir(a.RemotePath), 0o700); err != nil {
			return err
		}
		b, err := json.MarshalIndent(remote, "", "  ")
		if err != nil {
			return err
		}
		tmpFile, err := os.CreateTemp(filepath.Dir(a.RemotePath), filepath.Base(a.RemotePath)+".tmp.*")
		if err != nil {
			return err
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)
		if _, err := tmpFile.Write(b); err != nil {
			_ = tmpFile.Close()
			return err
		}
		if err := tmpFile.Chmod(0o600); err != nil {
			_ = tmpFile.Close()
			return err
		}
		if err := tmpFile.Close(); err != nil {
			return err
		}
		return os.Rename(tmpPath, a.RemotePath)
	})
}

func (a *App) loadRemoteHTTP() (*RemoteStore, error) {
	return a.loadRemoteHTTPFromURL(a.RemoteURL, a.authHeaderToken())
}

func (a *App) loadRemoteCloud() (*RemoteStore, error) {
	if a.cloudBaseURL() == "" {
		return nil, errors.New("cloud URL is not configured; set ENVSYNC_CLOUD_URL")
	}
	token, err := a.cloudAccessToken()
	if err != nil {
		return nil, err
	}
	return a.loadRemoteHTTPFromURL(a.cloudBaseURL(), token)
}

func (a *App) loadRemoteHTTPFromURL(baseURL, token string) (*RemoteStore, error) {
	var remote *RemoteStore
	err := a.withHTTPRetry(func() (bool, error) {
		req, err := http.NewRequest(http.MethodGet, strings.TrimSuffix(baseURL, "/")+"/v1/store", nil)
		if err != nil {
			return false, err
		}
		addAuthHeader(req, token)
		resp, err := a.httpClient().Do(req)
		if err != nil {
			return isRetryableNetworkError(err), err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			err := fmt.Errorf("remote GET failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
			return isRetryableStatus(resp.StatusCode), err
		}
		decoded, err := decodeRemoteStoreBody(resp.Body)
		if err != nil {
			return false, err
		}
		remote = decoded
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return remote, nil
}

func (a *App) saveRemoteHTTP(remote *RemoteStore, expectedRevision int) error {
	return a.saveRemoteHTTPToURL(a.RemoteURL, a.authHeaderToken(), remote, expectedRevision)
}

func (a *App) saveRemoteCloud(remote *RemoteStore, expectedRevision int) error {
	if a.cloudBaseURL() == "" {
		return errors.New("cloud URL is not configured; set ENVSYNC_CLOUD_URL")
	}
	token, err := a.cloudAccessToken()
	if err != nil {
		return err
	}
	return a.saveRemoteHTTPToURL(a.cloudBaseURL(), token, remote, expectedRevision)
}

func (a *App) saveRemoteHTTPToURL(baseURL, token string, remote *RemoteStore, expectedRevision int) error {
	remote.Revision = expectedRevision + 1
	body, err := encodeRemoteStoreBody(remote)
	if err != nil {
		return err
	}
	return a.withHTTPRetry(func() (bool, error) {
		req, err := http.NewRequest(http.MethodPut, strings.TrimSuffix(baseURL, "/")+"/v1/store", bytes.NewReader(body))
		if err != nil {
			return false, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("If-Match", strconv.Itoa(expectedRevision))
		addAuthHeader(req, token)
		resp, err := a.httpClient().Do(req)
		if err != nil {
			return isRetryableNetworkError(err), err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			err := fmt.Errorf("remote PUT failed: %s %s", resp.Status, strings.TrimSpace(string(respBody)))
			return isRetryableStatus(resp.StatusCode), err
		}
		return false, nil
	})
}

func attachCryptoMetadata(state *State, remote *RemoteStore) {
	remote.SaltB64 = state.SaltB64
	remote.KeyCheckB64 = state.KeyCheckB64
}

func validateRemoteCrypto(state *State, remote *RemoteStore) error {
	if remote.SaltB64 == "" && remote.KeyCheckB64 == "" {
		return nil
	}
	if remote.SaltB64 != state.SaltB64 || remote.KeyCheckB64 != state.KeyCheckB64 {
		return errors.New("remote store is encrypted with a different recovery phrase")
	}
	return nil
}

func withExclusiveFileLock(lockPath string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return err
	}
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	return fn()
}

func (a *App) httpClient() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (a *App) remoteRetryConfig() (int, time.Duration, time.Duration) {
	attempts := a.RemoteRetryMax
	if attempts <= 0 {
		attempts = 3
	}
	base := a.RemoteRetryBase
	if base <= 0 {
		base = 200 * time.Millisecond
	}
	maxDelay := a.RemoteRetryMaxD
	if maxDelay <= 0 {
		maxDelay = 2 * time.Second
	}
	if maxDelay < base {
		maxDelay = base
	}
	return attempts, base, maxDelay
}

func (a *App) withHTTPRetry(op func() (retryable bool, err error)) error {
	attempts, base, maxDelay := a.remoteRetryConfig()
	sleepFn := a.Sleep
	if sleepFn == nil {
		sleepFn = time.Sleep
	}

	var lastErr error
	for i := 1; i <= attempts; i++ {
		retryable, err := op()
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable || i == attempts {
			return err
		}
		backoff := base * time.Duration(1<<(i-1))
		if backoff > maxDelay {
			backoff = maxDelay
		}
		// 0-50% jitter helps spread retries under contention.
		jitter := time.Duration(rand.Int63n(int64(backoff)/2 + 1))
		sleepFn(backoff + jitter)
	}
	return lastErr
}

func isRetryableStatus(status int) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	return status >= 500 && status <= 599
}

func isRetryableNetworkError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}
