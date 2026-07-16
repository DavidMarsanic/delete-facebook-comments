package session

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// launchTimeout bounds how long we wait for Chrome's DevTools endpoint to
// come up after starting the process.
const launchTimeout = 15 * time.Second

// browserVersion mirrors the fields we need from Chrome's
// /json/version DevTools HTTP endpoint.
type browserVersion struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// NewBrowserContext attaches chromedp to Chrome running against this tool's
// own isolated profile directory — reusing an already-running instance from
// a previous invocation if one is still open, or launching a fresh one
// otherwise.
//
// Two earlier approaches were tried and rejected before landing here:
//   - Letting chromedp launch its own profile via its ExecAllocator adds
//     an --enable-automation flag (and sets navigator.webdriver), a
//     well-known bot-detection signal that got this tool CAPTCHA-walled
//     almost immediately against a real account.
//   - Attaching to the user's actual, already-logged-in default Chrome
//     profile (matching the old Python/Selenium version's design) turned
//     out to be blocked by Chrome itself: since a 2025 security hardening,
//     Chrome silently refuses to open a remote-debugging port at all
//     against the default/real profile, specifically to stop tools like
//     this one from attaching CDP to a user's live session. The flag is
//     accepted; the port just never opens.
//
// The isolated profile here sidesteps both: it's launched as a plain
// process (no automation flags), and — because it's a chromedp-only
// profile Chrome has never treated as "the active session" — remote
// debugging works on it reliably. To avoid making the user log into
// Facebook fresh in a profile with no history (itself a CAPTCHA trigger),
// its cookies are seeded from the user's real profile before the first
// launch, so it starts already authenticated. See seed.go.
//
// The window is visible (not headless) on purpose: a first run (or a
// cookie-seed miss) needs the user to log in by hand, and any CAPTCHA or
// checkpoint needs a human to see and resolve it. Call the returned cancel
// function to detach when done; it deliberately does not kill the browser,
// so the result of a run stays visible instead of vanishing immediately,
// and the next invocation can reuse the same window.
func NewBrowserContext(chromePath string) (context.Context, context.CancelFunc, error) {
	profileDir, err := ProfileDir()
	if err != nil {
		return nil, nil, err
	}

	port, wsURL, reused := reuseRunningInstance(profileDir)
	if !reused {
		seedCookiesFromRealProfile(profileDir)

		port, err = freePort()
		if err != nil {
			return nil, nil, fmt.Errorf("finding a free port for Chrome's debugger: %w", err)
		}

		cmd := exec.Command(chromePath,
			fmt.Sprintf("--remote-debugging-port=%d", port),
			"--user-data-dir="+profileDir,
			"--no-first-run",
			"--no-default-browser-check",
		)
		if err := cmd.Start(); err != nil {
			return nil, nil, fmt.Errorf("starting Chrome: %w", err)
		}

		wsURL, err = waitForDevTools(port, launchTimeout)
		if err != nil {
			_ = cmd.Process.Kill()
			return nil, nil, err
		}
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)

	// abortSetup is only for failures below, before anything is on screen
	// worth keeping: it fully tears down our tab and the connection.
	abortSetup := func() {
		ctxCancel()
		allocCancel()
	}

	if err := chromedp.Run(ctx, chromedp.Navigate("about:blank")); err != nil {
		abortSetup()
		return nil, nil, err
	}

	// cancel, returned to the caller, deliberately skips ctxCancel: calling
	// it closes the tab chromedp created (standard behavior for the
	// context returned by the top-level NewContext), and Chrome then
	// replaces it with a blank New Tab Page since a window can't have zero
	// tabs — which would erase whatever the run just showed the instant it
	// finished. allocCancel alone just drops our own WebSocket connection
	// and leaves the tab exactly as it is.
	cancel := func() {
		allocCancel()
	}

	// Extra tabs (Chrome's own initial blank tab, or a leftover from a
	// previous run) are cleaned up later, once the tool has actually
	// navigated somewhere — see activity.Engine.navigate. Trying to
	// identify "our" tab here, before it's done anything distinctive, is
	// unreliable: the remote allocator can reuse an existing blank tab as
	// its own working tab rather than opening a genuinely new one, and a
	// naive "close whatever existed before" approach ends up closing that
	// reused tab along with everything else.

	return ctx, cancel, nil
}

// reuseRunningInstance looks for a Chrome process already running against
// profileDir (left open by a previous invocation) and, if its DevTools
// endpoint still responds and it still has at least one tab open, returns
// its port and WebSocket URL. Best-effort: relies on pgrep/ps, so it simply
// reports "not found" on platforms without them (notably Windows), falling
// back to a fresh launch.
//
// A process that's alive but has zero tabs (the user closed the window
// directly, or a previous run's cleanup logic over-closed) is treated as
// unusable and killed rather than reused: chromedp's very first context on
// a new process only waits for Chrome to already have a tab to attach to —
// it does not create one itself — so a tabless instance can never be
// attached to and would just hang. Killing it clears the way for a fresh
// launch, which always gets Chrome's own freshly-created initial tab.
func reuseRunningInstance(profileDir string) (int, string, bool) {
	port, pid, ok := findRunningInstance(profileDir)
	if !ok {
		return 0, "", false
	}
	wsURL, err := waitForDevTools(port, 2*time.Second)
	if err != nil {
		return 0, "", false
	}
	if !hasOpenPage(port) {
		_ = exec.Command("kill", "-9", pid).Run()
		return 0, "", false
	}
	return port, wsURL, true
}

var debugPortPattern = regexp.MustCompile(`--remote-debugging-port=(\d+)`)

func findRunningInstance(profileDir string) (port int, pid string, ok bool) {
	out, err := exec.Command("pgrep", "-f", "user-data-dir="+profileDir).Output()
	if err != nil {
		return 0, "", false
	}
	pids := strings.Fields(string(out))
	if len(pids) == 0 {
		return 0, "", false
	}

	cmdOut, err := exec.Command("ps", "-p", pids[0], "-o", "command=").Output()
	if err != nil {
		return 0, "", false
	}
	m := debugPortPattern.FindStringSubmatch(string(cmdOut))
	if m == nil {
		return 0, "", false
	}
	port, err = strconv.Atoi(m[1])
	if err != nil {
		return 0, "", false
	}
	return port, pids[0], true
}

// hasOpenPage reports whether the Chrome instance at port has at least one
// open tab.
func hasOpenPage(port int) bool {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var targets []struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return false
	}
	for _, t := range targets {
		if t.Type == "page" {
			return true
		}
	}
	return false
}

// freePort asks the OS for an available TCP port by briefly binding to
// port 0, then releasing it for Chrome to use.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// waitForDevTools polls Chrome's /json/version endpoint until it responds
// or timeout elapses, then returns the WebSocket URL chromedp needs to
// attach to the browser-level DevTools target.
func waitForDevTools(port int, timeout time.Duration) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}

	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(200 * time.Millisecond)
			continue
		}
		var v browserVersion
		decErr := json.NewDecoder(resp.Body).Decode(&v)
		resp.Body.Close()
		if decErr != nil {
			lastErr = decErr
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if v.WebSocketDebuggerURL == "" {
			lastErr = fmt.Errorf("devtools endpoint returned no websocket URL")
			time.Sleep(200 * time.Millisecond)
			continue
		}
		return v.WebSocketDebuggerURL, nil
	}
	return "", fmt.Errorf("timed out waiting for Chrome's DevTools endpoint: %w", lastErr)
}
