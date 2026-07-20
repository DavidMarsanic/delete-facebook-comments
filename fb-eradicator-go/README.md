# fberadicator

A tool that clears sections of your own Facebook Activity Log — comments,
likes, page/interest likes, posts, or archives posts — by driving a real
Chrome/Chromium window over the Chrome DevTools Protocol. Usable either as a
CLI or through a small local web GUI.

This is a Go port of the original Python/Selenium scripts in the parent
repository, rewritten to build with a plain `go build ./...` (no cgo, no
external WebDriver binary) so it can go through a Go-only build pipeline.

## How it works

- Uses [chromedp](https://github.com/chromedp/chromedp) to drive Chrome
  directly over the DevTools protocol — no separate `chromedriver` binary to
  download or ship.
- Launches Chrome as a plain process — the same way double-clicking it would
  — against an isolated profile directory this tool owns, rather than using
  chromedp's own launcher. This matters: chromedp's default launcher adds an
  `--enable-automation` flag (and sets `navigator.webdriver`), a well-known
  bot-detection signal that gets a fresh profile CAPTCHA-walled by Facebook
  almost immediately.
- Attaching directly to your real, everyday Chrome profile was tried and
  rejected: since a 2025 security hardening, Chrome silently refuses to open
  a remote-debugging port at all against the default/real profile — the flag
  is accepted, the port just never opens — specifically to stop tools like
  this one from attaching to a live session.
- To avoid a fresh, no-history profile itself being a CAPTCHA trigger, the
  isolated profile's cookies are seeded from your real Chrome profile before
  the first launch, so it starts already logged into Facebook. Your password
  is never touched, read, or stored.
- An already-running instance from a previous invocation is reused where
  possible, rather than relaunching Chrome every time.
- Once signed in, it navigates to `facebook.com/me/allactivity` for the
  selected category and loops: select all (or select-in-range, with a date
  filter) → trigger the action (remove / trash / archive) → confirm → wait →
  repeat, until nothing is left. Every cycle — not just the first — patiently
  waits out a login prompt, a two-factor code request, or a CAPTCHA/checkpoint
  before continuing, since Facebook can interrupt mid-session on accounts
  doing bulk actions, not only at the very start.

## Prerequisites

- Go 1.21+ (only needed to build; the compiled binary has no runtime
  dependency on Go).
- Google Chrome, Chromium, Brave, Edge, or Arc already installed and, at
  some point, logged into Facebook (cookies are seeded from whichever
  profile Chrome's `Local State` reports as last used). This tool does not
  and cannot install a browser for you — it looks for one at startup and
  exits with a clear error if none is found.

## Build

```bash
go build -o fberadicator .
```

## Usage

### GUI

```bash
./fberadicator -gui
```

Starts a local web server and opens it in your default browser (a separate
page from the automation-controlled Chrome window). Pick a mode, an optional
date range, and a batch limit, then Start — progress streams live. Stop
interrupts the current run without closing the browser, so you can start
another one right after.

### CLI

```bash
./fberadicator -mode comments        # clear comments
./fberadicator -mode likes           # undo post likes
./fberadicator -mode interests       # unlike Pages/interests
./fberadicator -mode posts           # move your posts to trash
./fberadicator -mode archive-posts   # archive your posts instead of trashing them
```

Flags:

| Flag                  | Effect                                                                    |
|-----------------------|----------------------------------------------------------------------------|
| `-gui`                | Start the local web GUI instead of running from flags.                    |
| `-dry-run`            | Detect (and, with a date range, actually select) items only; never triggers or confirms. |
| `-limit N`            | Stop after N batches instead of continuing until nothing is left.         |
| `-date-from YYYY-MM-DD` | Only select items on/after this date (by Facebook's own day-group headers). |
| `-date-to YYYY-MM-DD`   | Only select items on/before this date.                                  |
| `-inspect`            | Read-only diagnostic dump of the page's selection UI.                     |
| `-inspect-dialog`     | Selects and opens the confirm dialog, then dumps it — never confirms.     |
| `-html <path>`        | Save the final page's full HTML to a file.                                |
| `-screenshot <path>`  | Save a PNG screenshot of the final page state.                            |

A date range scrolls the activity log to trigger Facebook's own lazy-loading
until it's loaded far enough back to cover the range (or hits the true end
of the list), then selects and processes only the items whose day header
falls inside it — a few items at a time per confirm, same as normal.

The browser window is left open after the tool finishes, so the result is
still visible instead of vanishing immediately — cancel only detaches, it
doesn't close the window.

## Safety notes

- Use this only on your own account. Bulk automated actions are exactly the
  pattern Facebook's abuse detection watches for.
- The tool pauses several seconds to half a minute (randomized) between
  batches rather than running at a fixed cadence, and checks for
  checkpoint/CAPTCHA/two-factor/rate-limit pages before every batch, not just
  at the start. If Facebook interrupts the flow, the tool waits and tells you
  to resolve it by hand in the open browser window — it will not try to
  guess its way past a CAPTCHA, and won't give up until you do (or it times
  out after 10 minutes).
- After selecting, the tool verifies the selection actually registered
  (checking `aria-checked`) before triggering the action, rather than
  trusting that a click succeeded just because it didn't error.
- There is no undo for removed comments, likes, or trashed posts once you
  confirm Facebook's own confirmation dialog.

## Status

Verified against a real account: login/cookie-seeding, CAPTCHA/checkpoint/
two-factor detection and patient waiting (both at the start and mid-session),
date-range selection with scroll-to-load (tested scrolling back from 2020 to
2016), the GUI's Start/Stop flow, and real deletions (selector confirmed
correct — item count dropped by exactly the expected amount) all work end to
end for the `comments` mode. The other modes (`likes`, `interests`, `posts`,
`archive-posts`) share the same engine and selector-porting approach but
haven't each been individually exercised against a live account yet —
Facebook's DOM can change, so treat the first run per mode as a supervised
trial, ideally with `-limit 1` first.
