package docs

import "strings"

// SectionsBySlug maps each Manifest slug to its page's table-of-contents
// entries (heading id + label): the exact data each page already passes to
// its own toc() sidebar widget, hoisted to package level so Search can walk
// every page's headings from one place instead of only their Manifest
// title/summary. Slugs with no written page yet (Stub-only topics like
// "bundle-size", and "" for Overview) have no entry, section search simply
// finds nothing for them.
var SectionsBySlug = map[string][]tocItem{
	"accessibility":     accessibilityTOCItems,
	"components":        componentsTOCItems,
	"concepts":          conceptsTOCItems,
	"develop-and-build": developAndBuildTOCItems,
	"dos-and-donts":     dosAndDontsTOCItems,
	"error-boundary":    errorBoundaryTOCItems,
	"head":              headTOCItems,
	"html-forms":        htmlFormsTOCItems,
	"installation":      installationTOCItems,
	"jsx-syntax":        jsxSyntaxTOCItems,
	"lucide-icons":      lucideIconsTOCItems,
	"patterns":          patternsTOCItems,
	"portal":            portalTOCItems,
	"project-structure": projectStructureTOCItems,
	"refs-and-dom":      refsAndDomTOCItems,
	"routing":           routingTOCItems,
	"security":          securityTOCItems,
	"signals":           signalsTOCItems,
	"store":             storeTOCItems,
	"style":             styleTOCItems,
	"troubleshooting":   troubleshootingTOCItems,
}

// SearchResult is one match: either a whole page (AnchorID == "") or a
// specific heading within a page (AnchorID set, Heading the heading text).
type SearchResult struct {
	Slug     string
	Title    string
	Heading  string
	AnchorID string
	Category string
}

// RoutePath is the /docs route for this result's page, with no anchor.
// This app's router treats any hash not starting with "#/" as a bare "/"
// navigation (see widgets.vane's scrollToHeading), so a heading's AnchorID
// can't be appended here as "#id", router.Navigate would break, it has to
// be applied as a separate scroll step after the route itself has mounted.
func (r SearchResult) RoutePath() string {
	if r.Slug == "" {
		return "/docs"
	}
	return "/docs/" + r.Slug
}

const (
	scoreTitleExact    = 100
	scoreTitlePrefix   = 80
	scoreTitleSubstr   = 60
	scoreHeadingSubstr = 40
	scoreSummarySubstr = 20
)

// Search matches query against every page's title/summary and every page's
// section headings (see SectionsBySlug), case-insensitive substring match.
// Results are ranked title match first, then heading, then summary, ties
// broken by Manifest order. Empty or whitespace-only query returns nil.
func Search(query string) []SearchResult {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}

	type scored struct {
		result SearchResult
		score  int
	}
	var hits []scored

	for _, t := range Manifest {
		title := strings.ToLower(t.Title)
		switch {
		case title == q:
			hits = append(hits, scored{SearchResult{Slug: t.Slug, Title: t.Title, Category: t.Category}, scoreTitleExact})
		case strings.HasPrefix(title, q):
			hits = append(hits, scored{SearchResult{Slug: t.Slug, Title: t.Title, Category: t.Category}, scoreTitlePrefix})
		case strings.Contains(title, q):
			hits = append(hits, scored{SearchResult{Slug: t.Slug, Title: t.Title, Category: t.Category}, scoreTitleSubstr})
		case strings.Contains(strings.ToLower(t.Summary), q):
			hits = append(hits, scored{SearchResult{Slug: t.Slug, Title: t.Title, Category: t.Category}, scoreSummarySubstr})
		}

		for _, item := range SectionsBySlug[t.Slug] {
			if strings.Contains(strings.ToLower(item.Label), q) {
				hits = append(hits, scored{
					SearchResult{Slug: t.Slug, Title: t.Title, Heading: item.Label, AnchorID: item.ID, Category: t.Category},
					scoreHeadingSubstr,
				})
			}
		}
	}

	// Stable sort: equal scores keep Manifest/heading declaration order.
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].score > hits[j-1].score; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}

	const maxResults = 20
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}

	out := make([]SearchResult, len(hits))
	for i, h := range hits {
		out[i] = h.result
	}
	return out
}
