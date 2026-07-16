package activity

import (
	"context"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// blockSignals are substrings of page text or URL that indicate Facebook has
// interrupted the flow with a checkpoint, CAPTCHA, or rate-limit notice.
// These are engineering safety nets, not something to click through
// automatically: the right move is always to stop and let the human decide.
var blockSignals = []string{
	"checkpoint",
	"suspicious activity",
	"confirm your identity",
	"help us confirm",
	"please try again later",
	"try again later",
	"temporarily blocked",
	"you're temporarily blocked",
	"unusual activity",
	"security check",
}

// checkBlocked inspects the current URL and visible page text for signals
// that Facebook has stepped in (CAPTCHA, checkpoint, rate limiting). It
// returns a human-readable reason when blocked, or "" when clear.
func (e *Engine) checkBlocked() (string, error) {
	var url, bodyText string
	ctx, cancel := context.WithTimeout(e.ctx, 5*time.Second)
	defer cancel()

	err := chromedp.Run(ctx,
		chromedp.Location(&url),
		chromedp.Evaluate(`document.body ? document.body.innerText.slice(0, 2000) : ""`, &bodyText),
	)
	if err != nil {
		// If we can't even inspect the page, don't fail the whole run over
		// it — treat as "not blocked" and let the calling step's own error
		// handling surface any real problem.
		return "", nil
	}

	lowerURL := strings.ToLower(url)
	lowerBody := strings.ToLower(bodyText)

	if strings.Contains(lowerURL, "checkpoint") || strings.Contains(lowerURL, "/recover/") {
		return "Facebook has redirected to a checkpoint/verification page", nil
	}

	for _, signal := range blockSignals {
		if strings.Contains(lowerBody, signal) {
			return "Facebook page contains a block/verification signal: \"" + signal + "\"", nil
		}
	}

	return "", nil
}
