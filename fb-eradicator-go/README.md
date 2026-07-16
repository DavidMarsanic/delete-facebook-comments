# fberadicator

A command-line tool that clears sections of your own Facebook Activity Log —
comments, likes, page/interest likes, posts, or archives posts — by driving a
real Chrome/Chromium window over the Chrome DevTools Protocol.

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
  selected category and loops: select all → trigger the action (remove /
  trash / archive) → confirm → wait → repeat, until nothing is left.

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

```bash
./fberadicator -mode comments        # clear comments
./fberadicator -mode likes           # undo post likes
./fberadicator -mode interests       # unlike Pages/interests
./fberadicator -mode posts           # move your posts to trash
./fberadicator -mode archive-posts   # archive your posts instead of trashing them
```

Flags:

| Flag              | Effect                                                                 |
|--------------------|-------------------------------------------------------------------------|
| `-dry-run`         | Detect items only; never clicks select-all, the action, or confirm.    |
| `-limit N`         | Stop after N batches instead of continuing until nothing is left.      |
| `-inspect`         | Read-only diagnostic dump of the page's selection UI.                  |
| `-inspect-dialog`  | Selects and opens the confirm dialog, then dumps it — never confirms.  |
| `-html <path>`     | Save the final page's full HTML to a file.                             |
| `-screenshot <path>` | Save a PNG screenshot of the final page state.                       |

The browser window is left open after the tool finishes, so the result is
still visible instead of vanishing immediately — cancel only detaches, it
doesn't close the window.

## Safety notes

- Use this only on your own account. Bulk automated actions are exactly the
  pattern Facebook's abuse detection watches for.
- The tool randomizes the delay between batches rather than running at a
  fixed cadence, and checks for checkpoint/CAPTCHA/rate-limit pages before
  each batch. If Facebook interrupts the flow, the tool stops and tells you
  to resolve it by hand in the open browser window — it will not try to
  guess its way past a CAPTCHA.
- There is no undo for removed comments, likes, or trashed posts once you
  confirm Facebook's own confirmation dialog.

## Status

Verified against a real account: login/cookie-seeding, CAPTCHA/checkpoint
detection and patient waiting, and a real single-item deletion (selector
confirmed correct — item count dropped by exactly one) all work end to end
for the `comments` mode. The other modes (`likes`, `interests`, `posts`,
`archive-posts`) share the same engine and selector-porting approach but
haven't each been individually exercised against a live account yet —
Facebook's DOM can change, so treat the first run per mode as a supervised
trial, ideally with `-limit 1` first.
