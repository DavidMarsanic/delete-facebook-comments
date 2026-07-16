// Package session launches a Chrome instance for chromedp to drive: an
// isolated profile directory this tool owns, pre-seeded with the cookies
// from the user's real Chrome profile so it starts already logged into
// Facebook.
package session

import (
	"os"
	"path/filepath"
)

const appDirName = "fb-eradicator"

// ProfileDir returns the path to this tool's persistent, isolated Chrome
// profile directory, creating it if it doesn't exist yet. It's kept
// separate from the user's real Chrome profile on purpose: modern Chrome
// (since a 2025 security hardening) refuses to open a remote-debugging
// port at all against the default/real profile — the flag is silently
// accepted but the port never actually opens — specifically to stop tools
// like this one from attaching CDP to a user's live, logged-in session.
// An isolated profile doesn't hit that restriction.
func ProfileDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, appDirName, "chrome-profile")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
