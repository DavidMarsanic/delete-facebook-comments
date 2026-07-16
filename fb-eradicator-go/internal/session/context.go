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

	cdpTarget "github.com/chromedp/cdproto/target"
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

// devtoolsTarget mirrors one entry from Chrome's /json/list endpoint.
type devtoolsTarget struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
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
// Rather than letting chromedp open its own new tab (ambiguous — it can
// either create a genuinely new one or silently reuse an existing blank
// one depending on browser state) and then hunting down and closing
// whatever's left over, this explicitly finds the one existing page tab
// (Chrome's own initial blank tab on a fresh launch, or a leftover from a
// previous run) and attaches chromedp directly to it via WithTargetID. The
// tool then navigates that same tab to Facebook. No second tab is ever
// created, so there's nothing to clean up afterwards.
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

	// Settle on exactly one existing page tab to reuse: if several are
	// already open (e.g. leftovers from before this reuse logic existed),
	// keep the first and close the rest, so we start from a known,
	// single-tab state before attaching chromedp to it.
	pageID := settleOnOnePage(port)

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)

	var ctx context.Context
	var ctxCancel context.CancelFunc
	if pageID != "" {
		ctx, ctxCancel = chromedp.NewContext(allocCtx, chromedp.WithTargetID(cdpTarget.ID(pageID)))
	} else {
		ctx, ctxCancel = chromedp.NewContext(allocCtx)
	}

	// abortSetup is only for failures below, before anything is on screen
	// worth keeping: it fully tears down our tab and the connection.
	abortSetup := func() {
		ctxCancel()
		allocCancel()
	}

	if err := chromedp.Run(ctx); err != nil {
		abortSetup()
		return nil, nil, err
	}

	// cancel, returned to the caller, deliberately skips ctxCancel: calling
	// it closes the tab chromedp is attached to (standard behavior for the
	// context returned by the top-level NewContext), and Chrome then
	// replaces it with a blank New Tab Page since a window can't have zero
	// tabs — which would erase whatever the run just showed the instant it
	// finished. allocCancel alone just drops our own WebSocket connection
	// and leaves the tab exactly as it is.
	cancel := func() {
		allocCancel()
	}

	return ctx, cancel, nil
}

// settleOnOnePage returns the ID of a single existing page tab to reuse,
// closing any others first. Returns "" if none exist yet (chromedp will
// fall back to its own default behavior in that case).
func settleOnOnePage(port int) string {
	targets, err := listTargets(port)
	if err != nil {
		return ""
	}

	var keep string
	for _, t := range targets {
		if t.Type != "page" {
			continue
		}
		if keep == "" {
			keep = t.ID
			continue
		}
		closeTargetHTTP(port, t.ID)
	}
	return keep
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
	targets, err := listTargets(port)
	if err != nil {
		return false
	}
	for _, t := range targets {
		if t.Type == "page" {
			return true
		}
	}
	return false
}

// listTargets fetches Chrome's own list of open tabs/pages over its
// DevTools HTTP API — plain JSON over HTTP, no CDP session required.
func listTargets(port int) ([]devtoolsTarget, error) {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var targets []devtoolsTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, err
	}
	return targets, nil
}

// closeTargetHTTP closes one tab via Chrome's DevTools HTTP API.
func closeTargetHTTP(port int, id string) {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/close/%s", port, id))
	if err != nil {
		return
	}
	resp.Body.Close()
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
