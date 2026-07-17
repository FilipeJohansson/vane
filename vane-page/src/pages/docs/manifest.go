package docs

// Topic is one entry in the docs sidebar. Slug is relative to /docs
// ("" is the overview index itself). Pages without hand-written content yet
// render through Stub and link out to the source markdown on GitHub.
type Topic struct {
	Slug     string
	Title    string
	Summary  string
	Category string
	// HasSource is true when docs/<slug>.md exists in the source repo. Stub
	// links straight to that file when true; when false (a topic that only
	// exists in this manifest so far, with no written guide anywhere yet) it
	// links to the docs/ folder instead of a dead file path.
	HasSource bool
}

// Manifest is the full docs table of contents, grouped by Category in
// display order. Most topics have a real in-app page, registered as its own
// router.Route in App.vane; slugs without one (e.g. "bundle-size") fall
// through to the catch-all ":slug" route and render via Stub.
var Manifest = []Topic{
	{Slug: "", Title: "Overview", Summary: "What Vane is and how the pieces fit together.", Category: "Getting Started", HasSource: true},
	{Slug: "concepts", Title: "Concepts", Summary: "The mental model: components, signals, no virtual DOM.", Category: "Getting Started", HasSource: true},

	{Slug: "installation", Title: "Installation", Summary: "Install the CLI and scaffold a project with vane init.", Category: "Start a New Project", HasSource: true},
	{Slug: "project-structure", Title: "Project Structure", Summary: "What vane init scaffolds: App.vane, src/pages, src/components, public/.", Category: "Start a New Project"},
	{Slug: "develop-and-build", Title: "Develop & Build", Summary: "The dev loop with vane run, production builds with vane build, deploying dist/.", Category: "Start a New Project"},
	{Slug: "signals", Title: "Signals & Reactivity", Summary: "Effect, OnDispose, ComputedOf, Untrack.", Category: "Start a New Project", HasSource: true},

	{Slug: "components", Title: "Components", Summary: "Functions, children, the controller pattern.", Category: "Build Your UI", HasSource: true},
	{Slug: "jsx-syntax", Title: "JSX Syntax", Summary: "Full prop/event table, inline for/if/switch.", Category: "Build Your UI", HasSource: true},
	{Slug: "style", Title: "Styles and CSS", Summary: "The core.Style struct and co-located CSS.", Category: "Build Your UI", HasSource: true},

	{Slug: "routing", Title: "Routing", Summary: "Router, params, layouts, ActiveLink.", Category: "Data & Navigation", HasSource: true},
	{Slug: "store", Title: "Global Store", Summary: "Package-level signals for shared state.", Category: "Data & Navigation", HasSource: true},
	{Slug: "refs-and-dom", Title: "Refs & DOM", Summary: "Reading and imperatively touching DOM nodes.", Category: "Data & Navigation", HasSource: true},

	{Slug: "portal", Title: "Portals", Summary: "Render into a DOM node outside the component tree.", Category: "Advanced", HasSource: true},
	{Slug: "head", Title: "Head Management", Summary: "Reactive document.title and meta tags.", Category: "Advanced", HasSource: true},

	{Slug: "dos-and-donts", Title: "Do's and Don'ts", Summary: "Vane-specific conventions: where JSX literals are allowed, Untrack for setup reads, and other easy mistakes.", Category: "Best Practices"},
	{Slug: "security", Title: "Security", Summary: "DangerousInnerHTML, escaping untrusted input, and other Vane security considerations.", Category: "Best Practices"},
	{Slug: "accessibility", Title: "Accessibility", Summary: "aria-*/role, focus management, live regions.", Category: "Best Practices", HasSource: true},
	{Slug: "error-boundary", Title: "Handle Errors", Summary: "Catch panics from a subtree without crashing the app.", Category: "Best Practices", HasSource: true},

	{Slug: "troubleshooting", Title: "Troubleshooting", Summary: "Known gotchas: reactive infinite loops, ref timing, and how to diagnose them.", Category: "Troubleshooting"},

	{Slug: "patterns", Title: "Patterns", Summary: "Patterns for common UI problems.", Category: "How-to Patterns", HasSource: true},
	{Slug: "bundle-size", Title: "Analyze Bundle Size", Summary: "Why the wasm binary is large, and how to inspect and trim it.", Category: "How-to Patterns"},
	{Slug: "data-fetching", Title: "Data Fetching", Summary: "Fetch data with a goroutine and net/http, no async library needed.", Category: "How-to Patterns"},
	{Slug: "html-forms", Title: "Build HTML Forms", Summary: "Controlled inputs, validation, and submit handling in Vane.", Category: "How-to Patterns"},
	{Slug: "lucide-icons", Title: "Add Lucide Icons", Summary: "Wire up the Lucide icon library, the same way this site's own Nav does.", Category: "How-to Patterns"},
}

// Categories returns the manifest grouped by Category, preserving the
// order categories first appear in.
func Categories() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range Manifest {
		if !seen[t.Category] {
			seen[t.Category] = true
			out = append(out, t.Category)
		}
	}
	return out
}

func TopicsIn(category string) []Topic {
	var out []Topic
	for _, t := range Manifest {
		if t.Category == category {
			out = append(out, t)
		}
	}
	return out
}

func TopicBySlug(slug string) (Topic, bool) {
	for _, t := range Manifest {
		if t.Slug == slug {
			return t, true
		}
	}
	return Topic{}, false
}

// PrevNext returns the manifest entries immediately before/after slug, in
// display order. hasPrev/hasNext report whether that side exists (the first
// topic has no prev, the last has no next).
func PrevNext(slug string) (prev, next Topic, hasPrev, hasNext bool) {
	i := -1
	for idx, t := range Manifest {
		if t.Slug == slug {
			i = idx
			break
		}
	}
	if i == -1 {
		return Topic{}, Topic{}, false, false
	}
	if i > 0 {
		prev, hasPrev = Manifest[i-1], true
	}
	if i < len(Manifest)-1 {
		next, hasNext = Manifest[i+1], true
	}
	return prev, next, hasPrev, hasNext
}
