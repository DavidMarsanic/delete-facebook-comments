# fberadicator

Clears sections of your own Facebook Activity Log in bulk — comments,
post likes, page/interest likes, posts (trashed or archived) — by driving
a real Chrome window, the same as if you were clicking through it by hand,
just automated and patient about it.

## Requirements

**Google Chrome, Chromium, Brave, Microsoft Edge, or Arc already installed**
and, at some point, logged into Facebook. fberadicator looks for one on
launch and tells you clearly if none is found — it doesn't install a
browser for you.

## Use

### GUI

Bare invocation (or `-gui`) starts a local web page: pick a mode, an
optional date range, and a batch limit, then Start. Progress streams live.
Stop interrupts the current run without closing the browser, so you can
start another one right after.

### Command line

```
fberadicator -mode comments        # clear comments
fberadicator -mode likes           # undo post likes
fberadicator -mode interests       # unlike Pages/interests
fberadicator -mode posts           # move your posts to trash
fberadicator -mode archive-posts   # archive your posts instead of trashing them
```

| Flag                    | Effect                                                                    |
|--------------------------|----------------------------------------------------------------------------|
| `-dry-run`               | Detect (and, with a date range, select) items only; never triggers or confirms. |
| `-limit N`               | Stop after N batches instead of continuing until nothing is left.         |
| `-date-from YYYY-MM-DD`  | Only act on items on/after this date.                                     |
| `-date-to YYYY-MM-DD`    | Only act on items on/before this date.                                    |

A date range works by scrolling Facebook's own Activity Log back far
enough to cover it, then acting only on what falls inside — same batching
either way.

The browser window stays open after the tool finishes, so you can see the
result — cancelling only detaches, it doesn't close the window.

## Safety notes

- **Use this only on your own account.** Bulk automated actions are
  exactly the pattern Facebook's abuse detection watches for.
- The tool paces itself with randomized pauses between batches, and pauses
  and waits for you if Facebook shows a checkpoint, CAPTCHA, or two-factor
  prompt mid-run — it won't try to guess past one, and won't give up
  waiting for you to clear it (up to 10 minutes).
- **There is no undo** for removed comments, undone likes, or trashed
  posts once Facebook's own confirmation dialog is accepted.
- Your Facebook password is never touched, read, or stored — the tool
  works against a browser session that's already logged in.

## Status

Verified end-to-end against a real account for the `comments` mode:
login, checkpoint/CAPTCHA handling, date-range selection, and real
deletions all confirmed working. The other modes (`likes`, `interests`,
`posts`, `archive-posts`) share the same engine but haven't each been
individually exercised against a live account yet — treat the first run
per mode as a supervised trial, ideally with `-limit 1` first.

## License

MIT — see [LICENSE](LICENSE).
