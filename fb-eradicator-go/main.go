// Command fberadicator clears sections of a Facebook account's Activity Log
// (comments, likes, interests, posts) by driving a real Chrome/Chromium
// window through the Chrome DevTools Protocol. It is a Go port of the
// original Python/Selenium scripts, intended to run against the user's own
// account.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"fberadicator/internal/activity"
	"fberadicator/internal/browser"
	"fberadicator/internal/session"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run() error {
	modeFlag := flag.String("mode", "", fmt.Sprintf("what to clear: %s", strings.Join(activity.Modes(), ", ")))
	dryRunFlag := flag.Bool("dry-run", false, "navigate and detect items only; never click delete/confirm")
	inspectFlag := flag.Bool("inspect", false, "navigate and dump diagnostic info about the page's selection UI; never click anything")
	inspectDialogFlag := flag.Bool("inspect-dialog", false, "select and trigger the action to open the confirm dialog, then dump its DOM; never clicks confirm")
	limitFlag := flag.Int("limit", 0, "stop after this many batches (0 = unlimited)")
	screenshotFlag := flag.String("screenshot", "", "save a PNG screenshot of the final page state to this path")
	htmlFlag := flag.String("html", "", "save the final page's full HTML to this path")
	flag.Usage = usage
	flag.Parse()

	if *modeFlag == "" {
		usage()
		return errors.New("missing -mode")
	}

	cat, ok := activity.Lookup(*modeFlag)
	if !ok {
		usage()
		return fmt.Errorf("unknown mode %q", *modeFlag)
	}

	chromePath, err := browser.Find()
	if err != nil {
		return fmt.Errorf("%w — install Google Chrome or Chromium and try again", err)
	}
	fmt.Println("Using browser:", chromePath)

	ctx, cancel, err := session.NewBrowserContext(chromePath)
	if err != nil {
		return fmt.Errorf("starting browser: %w", err)
	}
	defer cancel()

	switch {
	case *inspectFlag, *inspectDialogFlag:
		fmt.Printf("[inspect] Read-only: will navigate to %s and report on the page's selection UI. Nothing will be clicked or deleted.\n", cat.Name)
	case *dryRunFlag:
		fmt.Printf("[dry-run] This tool acts on your own Facebook account only. It will check for items in: %s (nothing will be deleted)\n", cat.Name)
	case *limitFlag > 0:
		fmt.Printf("This tool acts on your own Facebook account only. It will now clear up to %d batch(es) of: %s\n", *limitFlag, cat.Name)
	default:
		fmt.Printf("This tool acts on your own Facebook account only. It will now clear: %s\n", cat.Name)
	}

	engine := activity.New(ctx, cat, os.Stdout, *dryRunFlag, *limitFlag)

	runErr := runEngine(engine, *inspectFlag, *inspectDialogFlag)

	if *screenshotFlag != "" {
		if err := engine.Screenshot(*screenshotFlag); err != nil {
			fmt.Println("Warning: screenshot failed:", err)
		} else {
			fmt.Println("Screenshot saved to", *screenshotFlag)
		}
	}
	if *htmlFlag != "" {
		if err := engine.DumpHTML(*htmlFlag); err != nil {
			fmt.Println("Warning: HTML dump failed:", err)
		} else {
			fmt.Println("HTML saved to", *htmlFlag)
		}
	}

	if runErr != nil {
		if errors.Is(runErr, activity.ErrBlocked) {
			return runErr
		}
		return fmt.Errorf("run failed: %w", runErr)
	}
	return nil
}

func runEngine(engine *activity.Engine, inspect, inspectDialog bool) error {
	switch {
	case inspectDialog:
		return engine.InspectDialog()
	case inspect:
		return engine.Inspect()
	default:
		return engine.Run()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "fberadicator -mode <mode>")
	fmt.Fprintln(os.Stderr, "\nModes:")
	for _, m := range activity.Modes() {
		fmt.Fprintln(os.Stderr, "  "+m)
	}
}
