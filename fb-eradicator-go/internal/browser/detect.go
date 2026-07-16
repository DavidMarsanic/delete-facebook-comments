// Package browser locates an installed Chrome or Chromium executable
// across macOS, Linux, and Windows. The tool never bundles or installs a
// browser itself — chromedp drives whatever the user already has.
package browser

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ErrNotFound is returned when no Chrome/Chromium executable could be located.
var ErrNotFound = errors.New("no Chrome or Chromium installation found")

// candidatePaths returns absolute paths worth checking directly, before
// falling back to a PATH lookup. These cover the default install locations
// on each OS for Chrome, Chromium, and Edge (which is Chromium-based and
// speaks the same DevTools protocol).
func candidatePaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Arc.app/Contents/MacOS/Arc",
			filepath.Join(os.Getenv("HOME"), "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
		}
	case "windows":
		programFiles := os.Getenv("ProgramFiles")
		programFilesX86 := os.Getenv("ProgramFiles(x86)")
		localAppData := os.Getenv("LocalAppData")
		return []string{
			filepath.Join(programFiles, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(programFilesX86, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(localAppData, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(programFiles, "Chromium", "Application", "chrome.exe"),
			filepath.Join(programFiles, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(programFilesX86, "Microsoft", "Edge", "Application", "msedge.exe"),
		}
	default: // linux and other unix-likes
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
			"/usr/bin/microsoft-edge",
		}
	}
}

// pathNames returns executable names to look up on $PATH as a fallback,
// in priority order.
func pathNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"chrome.exe", "msedge.exe"}
	}
	return []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge"}
}

// Find locates a usable Chrome/Chromium executable and returns its path.
// It checks well-known install locations first, then falls back to PATH.
func Find() (string, error) {
	for _, p := range candidatePaths() {
		if p == "" {
			continue
		}
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}

	for _, name := range pathNames() {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}

	return "", ErrNotFound
}
