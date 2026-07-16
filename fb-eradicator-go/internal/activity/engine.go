// Package activity drives the actual Facebook Activity Log automation:
// navigate to a category, select everything, trigger the category's action,
// confirm it, and repeat until nothing is left.
package activity

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

const (
	activityURLTemplate = "https://www.facebook.com/me/allactivity?activity_history=false&category_key=%s&manage_mode=false&should_load_landing_page=false"
	checkboxSelector     = `[name="comet_activity_log_select_all_checkbox"]`

	maxStepRetries  = 3
	itemPollTimeout = 20 * time.Second

	loginWaitTimeout = 10 * time.Minute
	loginPollEvery   = 2 * time.Second
	loginRemindEvery = 15 * time.Second

	// navTimeout and actionTimeout bound every chromedp call made directly
	// against the long-lived engine context. Without a timeout, a stalled
	// DevTools connection (e.g. after sitting idle during a long login
	// wait) hangs the whole run forever instead of failing with a clear
	// error.
	navTimeout    = 20 * time.Second
	actionTimeout = 10 * time.Second
)

// ErrBlocked is returned when Facebook interrupts the flow with a checkpoint,
// CAPTCHA, or rate-limit signal. The caller should stop, not retry.
var ErrBlocked = errors.New("facebook has interrupted the flow and needs manual attention")

// Engine drives one category's automation loop against an already-open
// chromedp browser context.
type Engine struct {
	ctx    context.Context
	cat    Category
	out    io.Writer
	dryRun bool
	limit  int // 0 = unlimited; otherwise stop after this many batches
}

// New builds an Engine for the given category. ctx must be a live chromedp
// context (from chromedp.NewContext), already attached to a browser tab.
// When dryRun is true, the engine navigates, logs in, and reports whether
// items are present, but never clicks select-all, the action button, or a
// confirm dialog — nothing on the account is modified. limit, if non-zero,
// stops the run after that many batches instead of continuing until
// nothing is left.
func New(ctx context.Context, cat Category, out io.Writer, dryRun bool, limit int) *Engine {
	return &Engine{ctx: ctx, cat: cat, out: out, dryRun: dryRun, limit: limit}
}

// Run navigates to the category's activity log and repeatedly selects,
// triggers, and confirms the category's action until no items remain, a
// step fails after retries, or Facebook signals a block that needs a human.
func (e *Engine) Run() error {
	url := fmt.Sprintf(activityURLTemplate, e.cat.CategoryKey)
	if err := e.navigate(url); err != nil {
		return fmt.Errorf("navigating to activity log: %w", err)
	}

	present, err := e.ensureItemsReady()
	if err != nil {
		return err
	}

	cycle := 0
	for {
		if reason, err := e.checkBlocked(); err != nil {
			return err
		} else if reason != "" {
			return fmt.Errorf("%w: %s — resolve this in the open Chrome window, then re-run the tool", ErrBlocked, reason)
		}

		if !present {
			fmt.Fprintf(e.out, "No %s left. Done — %d batch(es) %s.\n", e.cat.Name, cycle, e.cat.Verb)
			return nil
		}

		if e.dryRun {
			fmt.Fprintf(e.out, "[dry-run] Items found for %s. Would select-all, trigger %q, and confirm here — stopping without changing anything.\n", e.cat.Name, e.cat.Verb)
			return nil
		}

		if err := e.retry("select all", e.selectAll); err != nil {
			return err
		}
		if err := e.retry("trigger action", e.triggerAction); err != nil {
			return err
		}

		// The confirmation dialog animates in; clicking immediately can
		// land before it's actually ready, especially on later cycles once
		// the page has more DOM churn behind it.
		time.Sleep(1 * time.Second)

		if err := e.retry("confirm", e.confirmAction); err != nil {
			return err
		}

		cycle++
		fmt.Fprintf(e.out, "Cycle %d: batch %s.\n", cycle, e.cat.Verb)

		if e.limit > 0 && cycle >= e.limit {
			fmt.Fprintf(e.out, "Reached the %d-batch limit. Stopping here.\n", e.limit)
			return nil
		}

		jitterSleep()

		present = e.waitForNextCycle()
		if !present {
			fmt.Fprintf(e.out, "No further %s detected. Done — %d batch(es) %s.\n", e.cat.Name, cycle, e.cat.Verb)
			return nil
		}
	}
}

// Inspect navigates to the category's activity log, waits past any
// login/CAPTCHA prompt the same way Run does, and dumps diagnostic
// information about the page's selection UI without clicking anything.
// It exists to pin down Facebook's current DOM when the tool's hardcoded
// selectors (ported from an older version of the site) go stale.
func (e *Engine) Inspect() error {
	url := fmt.Sprintf(activityURLTemplate, e.cat.CategoryKey)
	if err := e.navigate(url); err != nil {
		return fmt.Errorf("navigating to activity log: %w", err)
	}

	if _, err := e.ensureItemsReady(); err != nil {
		fmt.Fprintln(e.out, "Note: readiness wait ended with:", err)
	}

	const probe = `(() => {
		const checkboxes = Array.from(document.querySelectorAll('input[type="checkbox"]'));
		const selectLike = Array.from(document.querySelectorAll('[aria-label], [role="button"], span, div'))
			.filter(el => {
				const t = (el.textContent || '').trim();
				const a = el.getAttribute('aria-label') || '';
				return (t.length < 40 && /select all|select/i.test(t)) || /select all|select/i.test(a);
			})
			.slice(0, 15);
		return JSON.stringify({
			url: location.href,
			title: document.title,
			checkboxCount: checkboxes.length,
			checkboxSamples: checkboxes.slice(0, 5).map(c => c.outerHTML.slice(0, 300)),
			selectLike: selectLike.map(el => ({
				tag: el.tagName,
				aria: el.getAttribute('aria-label'),
				name: el.getAttribute('name'),
				text: (el.textContent || '').slice(0, 60),
			})),
		}, null, 2);
	})()`

	ctx, cancel := context.WithTimeout(e.ctx, actionTimeout)
	defer cancel()
	var result string
	if err := chromedp.Run(ctx, chromedp.Evaluate(probe, &result)); err != nil {
		return fmt.Errorf("probing page: %w", err)
	}
	fmt.Fprintln(e.out, result)
	return nil
}

// InspectDialog safely reproduces everything up to (but not including) the
// destructive confirm click: select-all, then trigger the action, which
// only opens Facebook's confirmation dialog — nothing is deleted until
// that dialog's own button is clicked, which this deliberately never does.
// It then dumps the dialog's actual DOM, so the real confirm-button
// selector can be verified against ground truth instead of guessed at
// against a live account.
func (e *Engine) InspectDialog() error {
	url := fmt.Sprintf(activityURLTemplate, e.cat.CategoryKey)
	if err := e.navigate(url); err != nil {
		return fmt.Errorf("navigating to activity log: %w", err)
	}

	if _, err := e.ensureItemsReady(); err != nil {
		fmt.Fprintln(e.out, "Note: readiness wait ended with:", err)
	}

	if err := e.retry("select all", e.selectAll); err != nil {
		return fmt.Errorf("select all: %w", err)
	}
	if err := e.retry("trigger action", e.triggerAction); err != nil {
		return fmt.Errorf("trigger action: %w", err)
	}

	time.Sleep(1 * time.Second)

	const probe = `(() => {
		const dialogs = Array.from(document.querySelectorAll('[role="dialog"]'));
		const removeLike = Array.from(document.querySelectorAll('[aria-label], [role="button"]'))
			.filter(el => {
				const a = el.getAttribute('aria-label') || '';
				const t = (el.textContent || '').trim();
				return /remove/i.test(a) || (t.length < 30 && /remove/i.test(t));
			});
		return JSON.stringify({
			url: location.href,
			dialogCount: dialogs.length,
			dialogHTML: dialogs.map(d => d.outerHTML.slice(0, 2000)),
			removeLikeCount: removeLike.length,
			removeLikeSamples: removeLike.slice(0, 10).map(el => ({
				tag: el.tagName,
				aria: el.getAttribute('aria-label'),
				role: el.getAttribute('role'),
				insideDialog: !!el.closest('[role="dialog"]'),
				outerHTML: el.outerHTML.slice(0, 400),
			})),
		}, null, 2);
	})()`

	ctx, cancel := context.WithTimeout(e.ctx, actionTimeout)
	defer cancel()
	var result string
	if err := chromedp.Run(ctx, chromedp.Evaluate(probe, &result)); err != nil {
		return fmt.Errorf("probing dialog: %w", err)
	}
	fmt.Fprintln(e.out, result)
	fmt.Fprintln(e.out, "\n[InspectDialog] Stopping here on purpose — nothing was confirmed/deleted. Close the dialog manually in the browser if you don't want to proceed.")
	return nil
}

// ensureItemsReady reports whether there's anything to process, but is
// patient about getting there: on a fresh profile the first navigation
// often lands on a login page or a cookie-consent prompt rather than the
// activity log, and those can still be in flight (client-side redirects,
// consent dialogs) at the instant navigation "completes." Rather than
// checking once and concluding "nothing here," this polls for the checkbox
// first, and only falls back to waiting on login if the page genuinely
// looks logged-out — so a real empty category is still reported quickly,
// while an unfinished login/cookie flow gets patience instead of a false
// "done."
func (e *Engine) ensureItemsReady() (bool, error) {
	if ok, err := e.pollCheckbox(itemPollTimeout); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}

	reason, waiting := e.notReadyReason()
	if !waiting {
		// Logged in, no block detected, page settled, no checkbox: genuinely nothing here.
		return false, nil
	}

	fmt.Fprintln(e.out, reason)
	fmt.Fprintln(e.out, "This tool will keep checking automatically — no need to re-run it.")

	deadline := time.Now().Add(loginWaitTimeout)
	lastRemind := time.Now()
	lastReason := reason
	for time.Now().Before(deadline) {
		time.Sleep(loginPollEvery)

		reason, waiting := e.notReadyReason()
		if !waiting {
			fmt.Fprintln(e.out, "Looks clear now. Reloading the activity log...")
			url := fmt.Sprintf(activityURLTemplate, e.cat.CategoryKey)
			if err := e.navigate(url); err != nil {
				return false, fmt.Errorf("re-navigating: %w", err)
			}
			return e.pollCheckbox(itemPollTimeout)
		}

		if reason != lastReason {
			fmt.Fprintln(e.out, reason)
			lastReason = reason
			lastRemind = time.Now()
		} else if time.Since(lastRemind) >= loginRemindEvery {
			fmt.Fprintln(e.out, "Still waiting — "+reason)
			lastRemind = time.Now()
		}
	}
	return false, fmt.Errorf("timed out waiting — last state: %s", lastReason)
}

// notReadyReason reports whether the browser is currently showing something
// that needs a human before the activity log can be used, and if so, a
// message describing what. It checks both the ordinary login form and
// Facebook's checkpoint/CAPTCHA/rate-limit signals (via checkBlocked) —
// a CAPTCHA page has neither a password field nor a "/login" URL, so
// without this check it looks identical to "successfully logged in,"
// which was silently misread as "nothing left to do" before.
func (e *Engine) notReadyReason() (string, bool) {
	if e.isLoggedOut() {
		return "Please finish logging into Facebook (and dismiss any cookie prompt) in the Chrome window that just opened.", true
	}
	if reason, err := e.checkBlocked(); err == nil && reason != "" {
		return "Facebook is showing a security check (" + reason + ") — please resolve it in the Chrome window.", true
	}
	return "", false
}

// isLoggedOut checks both the URL and the page's own DOM for signs we're
// not authenticated yet. Checking the DOM (for an actual password field)
// matters because Facebook doesn't always signal "not logged in" via a
// `/login`-style URL — it can render a login form client-side at the same
// URL you navigated to, which a URL-only check would miss entirely.
func (e *Engine) isLoggedOut() bool {
	var url, hasPasswordField string
	ctx, cancel := context.WithTimeout(e.ctx, 3*time.Second)
	defer cancel()

	err := chromedp.Run(ctx,
		chromedp.Location(&url),
		chromedp.Evaluate(`document.querySelector('input[type="password"], input[name="pass"]') ? "yes" : "no"`, &hasPasswordField),
	)
	if err != nil {
		// Can't tell right now — don't block progress on an inconclusive read.
		return false
	}

	lower := strings.ToLower(url)
	if strings.Contains(lower, "/login") || strings.Contains(lower, "login.php") {
		return true
	}
	return hasPasswordField == "yes"
}

// pollCheckbox reports whether the select-all checkbox is visible within
// the given timeout, i.e. whether there's anything left to process.
func (e *Engine) pollCheckbox(timeout time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(e.ctx, timeout)
	defer cancel()

	err := chromedp.Run(ctx, chromedp.WaitVisible(checkboxSelector, chromedp.ByQuery))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false, nil
	}
	return false, fmt.Errorf("checking for remaining items: %w", err)
}

// navigate loads url with a bounded timeout, so a stalled DevTools
// connection fails with a clear error instead of hanging the run forever.
// It also closes any other open tab (Chrome's own initial blank tab, or a
// leftover from a previous run) once our own tab is confirmed to be on
// Facebook, so exactly one tab — the useful one — stays open.
func (e *Engine) navigate(url string) error {
	ctx, cancel := context.WithTimeout(e.ctx, navTimeout)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(url)); err != nil {
		return err
	}
	e.closeOtherTabs()
	return nil
}

// closeOtherTabs closes every open tab that isn't currently on facebook.com.
// Tabs are identified by URL rather than by ID or creation order: with a
// remote allocator, chromedp can end up reusing an already-open blank tab
// as its own working tab instead of creating a genuinely new one, which
// makes any "close whatever existed before ours" logic unreliable — it can
// end up closing the very tab chromedp is using. Filtering on URL sidesteps
// that entirely, since our tab is unambiguously the one on Facebook.
func (e *Engine) closeOtherTabs() {
	ctx, cancel := context.WithTimeout(e.ctx, actionTimeout)
	defer cancel()

	targets, err := chromedp.Targets(ctx)
	if err != nil {
		return
	}
	for _, t := range targets {
		if t.Type != "page" || strings.Contains(t.URL, "facebook.com") {
			continue
		}
		_ = chromedp.Run(ctx, target.CloseTarget(t.TargetID))
	}
}

func (e *Engine) selectAll() error {
	ctx, cancel := context.WithTimeout(e.ctx, actionTimeout)
	defer cancel()
	return chromedp.Run(ctx, chromedp.Click(checkboxSelector, chromedp.ByQuery))
}

func (e *Engine) triggerAction() error {
	ctx, cancel := context.WithTimeout(e.ctx, actionTimeout)
	defer cancel()
	return chromedp.Run(ctx, chromedp.Click(e.cat.ActionSel, queryOpt(e.cat.ActionXPath)))
}

func (e *Engine) confirmAction() error {
	ctx, cancel := context.WithTimeout(e.ctx, actionTimeout)
	defer cancel()
	return chromedp.Run(ctx, chromedp.Click(e.cat.ConfirmSel, queryOpt(e.cat.ConfirmXPath)))
}

// waitForNextCycle polls for the select-all checkbox to become clickable
// again after a batch action, giving the page time to settle. It returns
// false if nothing reappears within the poll window, signalling "done."
func (e *Engine) waitForNextCycle() bool {
	deadline := time.Now().Add(itemPollTimeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(e.ctx, 1*time.Second)
		err := chromedp.Run(ctx, chromedp.WaitVisible(checkboxSelector, chromedp.ByQuery))
		cancel()
		if err == nil {
			return true
		}
		fmt.Fprintln(e.out, "Waiting for the next batch to become selectable...")
	}
	return false
}

// retry runs step up to maxStepRetries times with linear backoff, giving
// Facebook's UI time to catch up before treating a failure as fatal.
func (e *Engine) retry(label string, step func() error) error {
	var lastErr error
	for attempt := 1; attempt <= maxStepRetries; attempt++ {
		if err := step(); err != nil {
			lastErr = err
			fmt.Fprintf(e.out, "%s failed (attempt %d/%d): %v\n", label, attempt, maxStepRetries, err)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}
		return nil
	}
	return fmt.Errorf("%s: giving up after %d attempts: %w", label, maxStepRetries, lastErr)
}

// jitterSleep pauses for a randomized interval between batches. Fixed,
// metronomic delays are themselves a bot signal, so the interval is
// randomized rather than constant.
func jitterSleep() {
	minMs, maxMs := 1500, 3500
	d := time.Duration(minMs+rand.Intn(maxMs-minMs)) * time.Millisecond
	time.Sleep(d)
}

// Screenshot captures the current page as a PNG and writes it to path, so
// state can be verified visually instead of guessed at from DOM text dumps.
func (e *Engine) Screenshot(path string) error {
	ctx, cancel := context.WithTimeout(e.ctx, actionTimeout)
	defer cancel()

	var buf []byte
	if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&buf)); err != nil {
		return fmt.Errorf("capturing screenshot: %w", err)
	}
	return os.WriteFile(path, buf, 0o600)
}

// DumpHTML writes the current page's full HTML to path. This is the more
// useful of the two for diagnosing selector problems: it's exact, greppable
// text rather than a rendered image.
func (e *Engine) DumpHTML(path string) error {
	ctx, cancel := context.WithTimeout(e.ctx, actionTimeout)
	defer cancel()

	var html string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.documentElement.outerHTML`, &html)); err != nil {
		return fmt.Errorf("dumping HTML: %w", err)
	}
	return os.WriteFile(path, []byte(html), 0o600)
}

func queryOpt(xpath bool) chromedp.QueryOption {
	if xpath {
		return chromedp.BySearch
	}
	return chromedp.ByQuery
}
