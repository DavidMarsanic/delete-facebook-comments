package session

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// seedCookiesFromRealProfile copies the Cookies database from the user's
// real, everyday Chrome profile into this tool's isolated profile
// directory, so the isolated profile starts already logged into Facebook
// instead of needing a fresh login (which, on a brand-new profile with no
// browsing history, is exactly what triggers Facebook's CAPTCHA/checkpoint
// challenges). It's best-effort: if the real profile can't be found or
// copying fails for any reason, this quietly does nothing and the tool
// just falls back to asking the user to log in normally.
func seedCookiesFromRealProfile(isolatedProfileDir string) {
	realRoot, err := defaultChromeUserDataDir()
	if err != nil {
		return
	}

	realProfileName := lastUsedProfileName(realRoot)
	realProfileDir := filepath.Join(realRoot, realProfileName)

	dstDir := filepath.Join(isolatedProfileDir, "Default")
	if err := os.MkdirAll(dstDir, 0o700); err != nil {
		return
	}

	for _, name := range []string{"Cookies", "Cookies-journal", "Cookies-wal", "Cookies-shm"} {
		src := filepath.Join(realProfileDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		_ = copyFile(src, filepath.Join(dstDir, name))
	}
}

// lastUsedProfileName reads Chrome's Local State file to find which
// profile subdirectory (e.g. "Default", "Profile 3") the user was actually
// using, rather than assuming "Default" — Chrome creates a numbered
// profile per person added, and "Default" may not even be the one in use.
func lastUsedProfileName(userDataDir string) string {
	data, err := os.ReadFile(filepath.Join(userDataDir, "Local State"))
	if err != nil {
		return "Default"
	}

	var state struct {
		Profile struct {
			LastUsed string `json:"last_used"`
		} `json:"profile"`
	}
	if err := json.Unmarshal(data, &state); err != nil || state.Profile.LastUsed == "" {
		return "Default"
	}
	return state.Profile.LastUsed
}

// defaultChromeUserDataDir returns Chrome's default profile root — the
// same directory Chrome uses when launched with no --user-data-dir flag.
func defaultChromeUserDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Google", "Chrome"), nil
	case "windows":
		base := os.Getenv("LocalAppData")
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(base, "Google", "Chrome", "User Data"), nil
	default:
		return filepath.Join(home, ".config", "google-chrome"), nil
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
