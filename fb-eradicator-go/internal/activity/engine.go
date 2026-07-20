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

	"github.com/chromedp/chromedp"
)

const (
	activityURLTemplate = "https://www.facebook.com/me/allactivity?activity_history=false&category_key=%s&manage_mode=false&should_load_landing_page=false"
	checkboxSelector     = `[name="comet_activity_log_select_all_checkbox"]`

	maxStepRetries  = 5
	itemPollTimeout = 20 * time.Second

	// maxConsecutiveFailures bounds how many batches in a row can fail
	// (even after each one's own maxStepRetries attempts and a full page
	// reload) before Run gives up entirely. This is what lets a run left
	// unattended for hours shrug off an occasional stuck batch instead of
	// dying on the first one — a single flaky cycle in an otherwise long,
	// mostly-successful run resets the counter back to zero rather than
	// ending the run.
	maxConsecutiveFailures = 5

	loginWaitTimeout = 10 * time.Minute
	loginPollEvery   = 2 * time.Second
	loginRemindEvery = 15 * time.Second

	// navTimeout and actionTimeout bound every chromedp call made directly
	// against the long-lived engine context. Without a timeout, a stalled
	// DevTools connection (e.g. after sitting idle during a long login
	// wait) hangs the whole run forever instead of failing with a clear
	// error.
	navTimeout    = 20 * time.Second
	actionTimeout = 15 * time.Second

	// selectInRangeTimeout is larger than actionTimeout because a wide date
	// range can mean walking and clicking through months or years of
	// scrolled-in items in one JS call, not just a single element.
	selectInRangeTimeout = 60 * time.Second
)


// Options configures an Engine run. All fields have safe zero values: no
// dry run, no batch limit, no date restriction.
type Options struct {
	DryRun bool
	Limit  int // 0 = unlimited; otherwise stop after this many batches

	// DateFrom and DateTo restrict which items get selected, by the date
	// shown on Facebook's own day-group headers in the activity log
	// ("November 17, 2020"). Both are "YYYY-MM-DD"; either or both may be
	// empty for an open-ended range. Only items whose day header falls
	// within [DateFrom, DateTo] are selected — the plain select-all
	// checkbox is never used once a range is set, since that would ignore
	// the restriction entirely.
	DateFrom string
	DateTo   string
}

// Engine drives one category's automation loop against an already-open
// chromedp browser context.
type Engine struct {
	ctx  context.Context
	cat  Category
	out  io.Writer
	opts Options
}

// New builds an Engine for the given category. ctx must be a live chromedp
// context (from chromedp.NewContext), already attached to a browser tab.
// When opts.DryRun is true, the engine navigates, logs in, and reports
// whether items are present, but never clicks select-all, the action
// button, or a confirm dialog — nothing on the account is modified.
func New(ctx context.Context, cat Category, out io.Writer, opts Options) *Engine {
	return &Engine{ctx: ctx, cat: cat, out: out, opts: opts}
}

func (e *Engine) hasDateRange() bool {
	return e.opts.DateFrom != "" || e.opts.DateTo != ""
}

// ParseDateBound validates a date-range boundary before it's trusted enough
// to interpolate into a JS snippet (see jsDateBound). An empty string is
// valid and means "no bound." Callers (CLI flags, the GUI's JSON request)
// should call this on both DateFrom and DateTo before building Options.
func ParseDateBound(s string) error {
	if s == "" {
		return nil
	}
	_, err := time.Parse("2006-01-02", s)
	if err != nil {
		return fmt.Errorf("invalid date %q, expected YYYY-MM-DD: %w", s, err)
	}
	return nil
}

// Run navigates to the category's activity log and repeatedly selects,
// triggers, and confirms the category's action until no items remain, a
// step fails after retries, or Facebook signals a block that needs a human.
func (e *Engine) Run() error {
	url := fmt.Sprintf(activityURLTemplate, e.cat.CategoryKey)
	if err := e.navigate(url); err != nil {
		return fmt.Errorf("navigating to activity log: %w", err)
	}

	cycle := 0
	consecutiveFailures := 0
	for {
		// Checked at the top of every cycle, not just once before the loop:
		// Facebook can ask for your password again mid-session on accounts
		// doing bulk actions, not only at the very start, and a checkpoint
		// or CAPTCHA can just as easily appear between batches. Routing
		// every cycle through the same patient wait ensureItemsReady already
		// does for the initial login means any of these get waited through
		// consistently instead of only being handled at the start.
		present, err := e.ensureItemsReady()
		if err != nil {
			return err
		}

		if !present {
			fmt.Fprintf(e.out, "No %s left. Done — %d batch(es) %s.\n", e.cat.Name, cycle, e.cat.Verb)
			return nil
		}

		if e.opts.DryRun {
			if e.hasDateRange() {
				if err := e.loadUntilRangeCovered(); err != nil {
					return err
				}
				// Safe to actually run: this only clicks item checkboxes to
				// select them, which doesn't delete anything — it lets a
				// dry run report a real, verified count instead of just
				// "items found," confirming the date filter actually
				// matches what's expected before doing this for real.
				n, err := e.selectInRange()
				if err != nil {
					return err
				}
				fmt.Fprintf(e.out, "[dry-run] Would select %d item(s) of %s in range and trigger %q — stopping without changing anything.\n", n, e.cat.Name, e.cat.Verb)
				return nil
			}
			fmt.Fprintf(e.out, "[dry-run] Items found for %s. Would select-all, trigger %q, and confirm here — stopping without changing anything.\n", e.cat.Name, e.cat.Verb)
			return nil
		}

		done, cycleErr := e.processCycle()
		if cycleErr != nil {
			if e.ctx.Err() != nil {
				// Stopped (or the connection dropped) — no amount of
				// reloading fixes a cancelled context, so don't try.
				return fmt.Errorf("stopped: %w", e.ctx.Err())
			}
			consecutiveFailures++
			fmt.Fprintf(e.out, "Batch failed (%d/%d consecutive failures): %v\n", consecutiveFailures, maxConsecutiveFailures, cycleErr)
			if consecutiveFailures >= maxConsecutiveFailures {
				return fmt.Errorf("giving up after %d consecutive batch failures: %w", maxConsecutiveFailures, cycleErr)
			}
			fmt.Fprintln(e.out, "Reloading the activity log and trying again...")
			if err := e.navigate(url); err != nil {
				return fmt.Errorf("re-navigating after failure: %w", err)
			}
			time.Sleep(5 * time.Second)
			continue
		}
		consecutiveFailures = 0

		if done {
			fmt.Fprintf(e.out, "No %s left in the selected date range (among items currently loaded). Done — %d batch(es) %s.\n", e.cat.Name, cycle, e.cat.Verb)
			return nil
		}

		cycle++
		fmt.Fprintf(e.out, "Cycle %d: batch %s.\n", cycle, e.cat.Verb)

		if e.opts.Limit > 0 && cycle >= e.opts.Limit {
			fmt.Fprintf(e.out, "Reached the %d-batch limit. Stopping here.\n", e.opts.Limit)
			return nil
		}

		e.jitterSleep()
	}
}

// processCycle selects (all, or everything in the configured date range),
// triggers the category's action, and confirms it — one batch. done=true
// (with err=nil) means date-range mode found nothing left to select among
// currently-loaded items — a normal completion, not a failure.
func (e *Engine) processCycle() (done bool, err error) {
	if e.hasDateRange() {
		if err := e.loadUntilRangeCovered(); err != nil {
			return false, err
		}
		var selected int
		selectStep := func() error {
			n, err := e.selectInRange()
			if err != nil {
				return err
			}
			selected = n
			return nil
		}
		if err := e.retry("select in range", selectStep); err != nil {
			return false, err
		}
		if selected == 0 {
			return true, nil
		}
		fmt.Fprintf(e.out, "Selected %d item(s) in range.\n", selected)
	} else if err := e.retry("select all", e.selectAll); err != nil {
		return false, err
	}

	if err := e.retry("trigger action", e.triggerAction); err != nil {
		return false, err
	}

	// The confirmation dialog animates in; clicking immediately can land
	// before it's actually ready, especially on later cycles once the page
	// has more DOM churn behind it.
	time.Sleep(1 * time.Second)

	if err := e.retry("confirm", e.confirmAction); err != nil {
		return false, err
	}

	return false, nil
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
		const scrollable = Array.from(document.querySelectorAll('*'))
			.filter(el => el.scrollHeight > el.clientHeight + 50 && getComputedStyle(el).overflowY !== 'visible')
			.slice(0, 5)
			.map(el => ({
				tag: el.tagName,
				class: (el.className || '').toString().slice(0, 80),
				scrollHeight: el.scrollHeight,
				clientHeight: el.clientHeight,
			}));
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
			scrollableCandidates: scrollable,
			windowScrollable: document.body.scrollHeight > window.innerHeight,
			bodyScrollHeight: document.body.scrollHeight,
			windowInnerHeight: window.innerHeight,
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

	e.alert(reason)
	fmt.Fprintln(e.out, "This tool will keep checking automatically — no need to re-run it.")

	deadline := time.Now().Add(loginWaitTimeout)
	lastRemind := time.Now()
	lastReason := reason
	for time.Now().Before(deadline) {
		time.Sleep(loginPollEvery)

		if e.ctx.Err() != nil {
			return false, fmt.Errorf("stopped: %w", e.ctx.Err())
		}

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
			// A genuinely new kind of interruption (e.g. login turned into
			// a CAPTCHA) — worth a fresh alert, not just a silent update.
			e.alert(reason)
			lastReason = reason
			lastRemind = time.Now()
		} else if time.Since(lastRemind) >= loginRemindEvery {
			fmt.Fprintln(e.out, "Still waiting — "+reason)
			lastRemind = time.Now()
		}
	}
	return false, fmt.Errorf("timed out waiting — last state: %s", lastReason)
}

// attentionMarker prefixes a log line the GUI's frontend watches for to
// trigger an audible alert — e.g. a Facebook password/2FA re-prompt during
// an otherwise-unattended run is easy to miss without one. The leading
// ASCII bell character gives CLI users the same nudge: most terminals beep
// on \a if they haven't disabled it.
const attentionMarker = "[ATTENTION]"

func (e *Engine) alert(reason string) {
	fmt.Fprintf(e.out, "\a%s %s\n", attentionMarker, reason)
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
func (e *Engine) navigate(url string) error {
	ctx, cancel := context.WithTimeout(e.ctx, navTimeout)
	defer cancel()
	return chromedp.Run(ctx, chromedp.Navigate(url))
}

func (e *Engine) selectAll() error {
	ctx, cancel := context.WithTimeout(e.ctx, actionTimeout)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Click(checkboxSelector, chromedp.ByQuery)); err != nil {
		return err
	}
	return e.verifySelected()
}

// verifySelected confirms the select-all click actually registered rather
// than trusting a successful click alone: chromedp.Click only reports
// whether the click event was dispatched, not whether Facebook's UI
// actually flipped the checkbox to selected, so a click that lands during
// a re-render can silently no-op.
func (e *Engine) verifySelected() error {
	ctx, cancel := context.WithTimeout(e.ctx, 3*time.Second)
	defer cancel()

	var checked string
	err := chromedp.Run(ctx, chromedp.AttributeValue(checkboxSelector, "aria-checked", &checked, nil, chromedp.ByQuery))
	if err != nil {
		return fmt.Errorf("checking selection state: %w", err)
	}
	if checked != "true" {
		return fmt.Errorf("select-all did not register as checked (aria-checked=%q)", checked)
	}
	return nil
}

// dateRangeSelectScript walks the page in document order, tracking the most
// recent day-group header ("November 17, 2020", an <h2>) as it goes, and
// clicks every not-yet-selected item checkbox whose header falls within
// [from, to]. Walking in document order (rather than querying headers and
// checkboxes separately) is what lets a checkbox be matched to the header
// that precedes it, since Facebook's markup doesn't attach the date to the
// item element itself. querySelectorAll with a comma-separated selector
// returns matches in document order, which this depends on.
const dateRangeSelectScript = `(() => {
	const dateRe = /^(January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{1,2},\s+\d{4}$/;
	const from = %s;
	const to = %s;
	const nodes = Array.from(document.querySelectorAll('h2, input[name="comet_activity_log_item_checkbox"]'));
	let currentDate = null;
	let clicked = 0;
	for (const node of nodes) {
		if (node.tagName === 'H2') {
			const text = (node.textContent || '').trim();
			if (dateRe.test(text)) {
				currentDate = new Date(text);
			}
			continue;
		}
		if (!currentDate) continue;
		if (from !== null && currentDate < from) continue;
		if (to !== null && currentDate > to) continue;
		if (node.getAttribute('aria-checked') !== 'true') {
			node.click();
			clicked++;
		}
	}
	return clicked;
})()`

const (
	scrollWait        = 3 * time.Second
	maxScrollAttempts = 300             // generous upper bound for very old/large accounts
	maxScrollDuration = 5 * time.Minute // safety net alongside the attempt cap
	maxStalls         = 3               // consecutive no-growth checks before concluding "true end"
)

// oldestLoadedDateScript returns the day-group header text of the
// oldest item currently rendered — the last date header in document
// order, since Facebook's activity log lists newest first — or "" if
// none are loaded yet.
const oldestLoadedDateScript = `(() => {
	const dateRe = /^(January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{1,2},\s+\d{4}$/;
	const headers = Array.from(document.querySelectorAll('h2'))
		.map(h => (h.textContent || '').trim())
		.filter(t => dateRe.test(t));
	return headers.length ? headers[headers.length - 1] : '';
})()`

// loadUntilRangeCovered scrolls the activity log to trigger Facebook's own
// lazy-loading until the oldest currently-rendered item is at or before
// DateFrom (so the whole requested range is covered), or until scrolling
// stops loading anything new (the true end of the list). A no-op when
// DateFrom isn't set, since there's no lower bound to scroll back to.
func (e *Engine) loadUntilRangeCovered() error {
	if e.opts.DateFrom == "" {
		return nil
	}
	from, err := time.Parse("2006-01-02", e.opts.DateFrom)
	if err != nil {
		return fmt.Errorf("parsing date-from: %w", err)
	}

	deadline := time.Now().Add(maxScrollDuration)
	lastCount := -1
	stalls := 0

	for attempt := 0; attempt < maxScrollAttempts && time.Now().Before(deadline); attempt++ {
		oldest, err := e.oldestLoadedDate()
		if err != nil {
			return fmt.Errorf("checking loaded date range: %w", err)
		}
		if oldest != "" {
			oldestDate, err := time.Parse("January 2, 2006", oldest)
			if err == nil && !oldestDate.After(from) {
				return nil // loaded far enough back to cover the requested range
			}
		}

		count, err := e.countLoadedItems()
		if err != nil {
			return fmt.Errorf("counting loaded items: %w", err)
		}
		if count == lastCount {
			stalls++
			// A single unchanged count can just mean the network fetch for
			// the next page hadn't finished yet when we checked — require a
			// few in a row, not one, before concluding this is genuinely
			// the end of the list rather than a slow load.
			if stalls >= maxStalls {
				fmt.Fprintf(e.out, "No more %s loaded after %d attempts — this looks like the true end of the list (oldest: %s).\n", e.cat.Name, stalls, orNone(oldest))
				return nil
			}
		} else {
			stalls = 0
		}
		lastCount = count

		if attempt%5 == 0 {
			fmt.Fprintf(e.out, "Scrolling to load older %s (oldest loaded so far: %s)...\n", e.cat.Name, orNone(oldest))
		}

		if err := e.scrollToBottom(); err != nil {
			return fmt.Errorf("scrolling: %w", err)
		}
		time.Sleep(scrollWait)
	}
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "none yet"
	}
	return s
}

func (e *Engine) oldestLoadedDate() (string, error) {
	ctx, cancel := context.WithTimeout(e.ctx, actionTimeout)
	defer cancel()
	var date string
	err := chromedp.Run(ctx, chromedp.Evaluate(oldestLoadedDateScript, &date))
	return date, err
}

func (e *Engine) countLoadedItems() (int, error) {
	ctx, cancel := context.WithTimeout(e.ctx, actionTimeout)
	defer cancel()
	var count int
	err := chromedp.Run(ctx, chromedp.Evaluate(
		`document.querySelectorAll('input[name="comet_activity_log_item_checkbox"]').length`, &count))
	return count, err
}

func (e *Engine) scrollToBottom() error {
	ctx, cancel := context.WithTimeout(e.ctx, actionTimeout)
	defer cancel()
	return chromedp.Run(ctx, chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil))
}

// selectInRange clicks every currently-loaded item whose day-group header
// falls within the configured date range, and reports how many it clicked.
// It never touches the select-all checkbox, since that would select
// everything regardless of date.
func (e *Engine) selectInRange() (int, error) {
	// A wide date range can mean scrolling loads months or years of items
	// before selection runs, so this walks (and clicks within) a much
	// larger DOM than a single page of results — actionTimeout's 10s,
	// sized for a single click, isn't enough headroom for that.
	ctx, cancel := context.WithTimeout(e.ctx, selectInRangeTimeout)
	defer cancel()

	script := fmt.Sprintf(dateRangeSelectScript, jsDateBound(e.opts.DateFrom), jsDateBound(e.opts.DateTo))
	var clicked int
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &clicked)); err != nil {
		return 0, fmt.Errorf("selecting items in date range: %w", err)
	}
	return clicked, nil
}

// jsDateBound renders a "YYYY-MM-DD" string (already validated by the
// caller — see ParseDateBound) as a JS Date literal, or the literal null
// for an open-ended bound.
func jsDateBound(s string) string {
	if s == "" {
		return "null"
	}
	return fmt.Sprintf("new Date(%q)", s)
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


// retry runs step up to maxStepRetries times with linear backoff, giving
// Facebook's UI time to catch up before treating a failure as fatal.
func (e *Engine) retry(label string, step func() error) error {
	var lastErr error
	for attempt := 1; attempt <= maxStepRetries; attempt++ {
		if err := step(); err != nil {
			lastErr = err
			// Stop (or the browser connection dropping) cancels e.ctx —
			// once that's happened, every remaining attempt is guaranteed
			// to fail the same way, so further retries (and their backoff
			// sleeps) just delay reporting what's already a foregone
			// conclusion. Bail immediately instead of grinding through the
			// rest of the budget.
			if e.ctx.Err() != nil {
				return fmt.Errorf("%s: stopped: %w", label, e.ctx.Err())
			}
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
// randomized rather than constant — and deliberately generous (several
// seconds up to half a minute) rather than just enough to be polite, since
// a tight, consistent cadence across many batches is itself a pattern
// Facebook's abuse detection watches for.
func (e *Engine) jitterSleep() {
	minMs, maxMs := 6000, 25000
	d := time.Duration(minMs+rand.Intn(maxMs-minMs)) * time.Millisecond
	fmt.Fprintf(e.out, "Pausing %s before the next batch...\n", d.Round(time.Second))
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
