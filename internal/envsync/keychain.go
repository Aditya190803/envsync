package envsync

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func (a *App) keychainServiceName() string {
	if s := strings.TrimSpace(os.Getenv("ENVSYNC_KEYCHAIN_SERVICE")); s != "" {
		return s
	}
	if s := strings.TrimSpace(a.KeychainService); s != "" {
		return s
	}
	return "envsync-recovery-phrase"
}

func (a *App) sessionServiceName() string {
	if s := strings.TrimSpace(os.Getenv("ENVSYNC_SESSION_SERVICE")); s != "" {
		return s
	}
	if s := strings.TrimSpace(a.SessionService); s != "" {
		return s
	}
	return "envsync-cloud-session"
}

func (a *App) phraseFromKeychain() (string, error) {
	service := a.keychainServiceName()
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("security", "find-generic-password", "-a", "envsync", "-s", service, "-w").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	case "linux":
		out, err := exec.Command("secret-tool", "lookup", "service", service, "account", "envsync").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	default:
		return "", errors.New("keychain not supported on this OS")
	}
}

func (a *App) storePhraseKeychain(phrase string) error {
	service := a.keychainServiceName()
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("security", "add-generic-password", "-a", "envsync", "-s", service, "-w", phrase, "-U")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("keychain save failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	case "linux":
		cmd := exec.Command("secret-tool", "store", "--label=envsync recovery phrase", "service", service, "account", "envsync")
		cmd.Stdin = strings.NewReader(phrase)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("keychain save failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return errors.New("keychain save not supported on this OS")
	}
}

func (a *App) clearPhraseKeychain() error {
	service := a.keychainServiceName()
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("security", "delete-generic-password", "-a", "envsync", "-s", service)
		if out, err := cmd.CombinedOutput(); err != nil {
			msg := strings.TrimSpace(string(out))
			if strings.Contains(strings.ToLower(msg), "could not be found") {
				return nil
			}
			return fmt.Errorf("keychain clear failed: %s", msg)
		}
		return nil
	case "linux":
		cmd := exec.Command("secret-tool", "clear", "service", service, "account", "envsync")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("keychain clear failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return errors.New("keychain clear not supported on this OS")
	}
}

func (a *App) sessionFromKeychain() (string, error) {
	service := a.sessionServiceName()
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("security", "find-generic-password", "-a", "envsync", "-s", service, "-w").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	case "linux":
		out, err := exec.Command("secret-tool", "lookup", "service", service, "account", "envsync").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	default:
		return "", errors.New("keychain not supported on this OS")
	}
}

func (a *App) storeSessionKeychain(sessionJSON string) error {
	service := a.sessionServiceName()
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("security", "add-generic-password", "-a", "envsync", "-s", service, "-w", sessionJSON, "-U")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("keychain save failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	case "linux":
		cmd := exec.Command("secret-tool", "store", "--label=envsync cloud session", "service", service, "account", "envsync")
		cmd.Stdin = strings.NewReader(sessionJSON)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("keychain save failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return errors.New("keychain save not supported on this OS")
	}
}

func (a *App) clearSessionKeychain() error {
	service := a.sessionServiceName()
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("security", "delete-generic-password", "-a", "envsync", "-s", service)
		if out, err := cmd.CombinedOutput(); err != nil {
			msg := strings.TrimSpace(string(out))
			if strings.Contains(strings.ToLower(msg), "could not be found") {
				return nil
			}
			return fmt.Errorf("keychain clear failed: %s", msg)
		}
		return nil
	case "linux":
		cmd := exec.Command("secret-tool", "clear", "service", service, "account", "envsync")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("keychain clear failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return errors.New("keychain clear not supported on this OS")
	}
}
