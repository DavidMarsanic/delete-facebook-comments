package activity

import "sort"

// Category describes one Facebook Activity Log section this tool can clear,
// along with the selectors needed to trigger and confirm the action.
//
// Selectors are ported 1:1 from the code paths actually exercised by the
// original Python/Selenium scripts (the "final confirmation" functions that
// were live in each script's loop), not the unused alternate functions left
// over from earlier iterations of those scripts.
type Category struct {
	Name        string // CLI-facing mode name
	CategoryKey string // Facebook's category_key query param
	Verb        string // past-tense verb for log messages, e.g. "removed"

	ActionSel    string // selector for the button that starts the action (Remove/Trash/Archive)
	ActionXPath  bool   // true if ActionSel is an XPath expression rather than CSS
	ConfirmSel   string // selector for the confirmation button in the resulting dialog
	ConfirmXPath bool   // true if ConfirmSel is an XPath expression rather than CSS
}

// registry holds all supported categories, keyed by CLI mode name.
var registry = map[string]Category{
	"comments": {
		Name:        "comments",
		CategoryKey: "COMMENTSCLUSTER",
		Verb:        "removed",
		ActionSel:   "//span[text()='Remove']",
		ActionXPath: true,
		ConfirmSel:  `div[role="dialog"] div[aria-label="Remove"][role="button"]`,
	},
	"likes": {
		Name:        "likes",
		CategoryKey: "LIKEDPOSTS",
		Verb:        "unliked",
		ActionSel:   "//span[text()='Remove']",
		ActionXPath: true,
		ConfirmSel:  `div[role="dialog"] div[aria-label="Remove"][role="button"]`,
	},
	"interests": {
		Name:        "interests",
		CategoryKey: "LIKEDINTERESTS",
		Verb:        "removed",
		ActionSel:   "//span[text()='Remove']",
		ActionXPath: true,
		ConfirmSel:  `div[role="dialog"] div[aria-label="Remove"][role="button"]`,
	},
	"posts": {
		Name:         "posts",
		CategoryKey:  "MANAGEPOSTSPHOTOSANDVIDEOS",
		Verb:         "moved to trash",
		ActionSel:    `div[aria-label="Trash"][role="button"]`,
		ConfirmSel:   "//div[@aria-label='Move to trash'][@role='button']",
		ConfirmXPath: true,
	},
	"archive-posts": {
		Name:         "archive-posts",
		CategoryKey:  "MANAGEPOSTSPHOTOSANDVIDEOS",
		Verb:         "archived",
		ActionSel:    `div[aria-label="Archive"][role="button"]`,
		ConfirmSel:   "//div[@aria-label='Move to archive'][@role='button']",
		ConfirmXPath: true,
	},
}

// Lookup returns the Category registered under the given CLI mode name.
func Lookup(mode string) (Category, bool) {
	c, ok := registry[mode]
	return c, ok
}

// Modes returns all supported CLI mode names, sorted for stable help text.
func Modes() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
