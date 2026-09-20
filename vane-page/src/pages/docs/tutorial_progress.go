package docs

import (
	"strings"

	"github.com/filipejohansson/vane/core"
)

// tutorialProgressKey is the localStorage key the tutorial's completion
// checkboxes persist to - a comma-separated set of done unit slugs.
const tutorialProgressKey = "vane-tutorial-progress"

// TutorialUnitSlugs lists the 5 units in build order - the denominator for
// "N of 5 complete", and what a completed set is checked against.
var TutorialUnitSlugs = []string{
	"tutorial-1-scaffold",
	"tutorial-2-data-model",
	"tutorial-3-add-todos",
	"tutorial-4-toggle-remove",
	"tutorial-5-filter-routes",
}

func loadTutorialProgress() map[string]bool {
	done := map[string]bool{}
	raw, ok := core.LocalStorageGet(tutorialProgressKey)
	if !ok || raw == "" {
		return done
	}
	for _, slug := range strings.Split(raw, ",") {
		done[slug] = true
	}
	return done
}

func saveTutorialProgress(done map[string]bool) {
	var slugs []string
	for _, slug := range TutorialUnitSlugs {
		if done[slug] {
			slugs = append(slugs, slug)
		}
	}
	core.LocalStorageSet(tutorialProgressKey, strings.Join(slugs, ","))
}

// TutorialProgress is package-level shared state (same pattern as Global
// Store: a plain core.NewSignal at package scope, documented in Store.vane)
// so the sidebar, the tutorial landing page, and whichever unit page is
// open all reflect the same completion set the instant one of them changes
// it, with no prop-drilling between DocsLayout and the content pages it
// hosts.
var TutorialProgress = core.NewSignal(loadTutorialProgress())

// ToggleTutorialUnit flips slug's done state and persists the result.
func ToggleTutorialUnit(slug string) {
	done := TutorialProgress.Get()
	next := make(map[string]bool, len(done)+1)
	for k, v := range done {
		next[k] = v
	}
	next[slug] = !next[slug]
	saveTutorialProgress(next)
	TutorialProgress.Set(next)
}

func TutorialUnitDone(slug string) bool {
	return TutorialProgress.Get()[slug]
}

func TutorialCompletedCount() int {
	done := TutorialProgress.Get()
	n := 0
	for _, slug := range TutorialUnitSlugs {
		if done[slug] {
			n++
		}
	}
	return n
}
