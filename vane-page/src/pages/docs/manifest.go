package docs

// Topic is one entry in the docs sidebar. Slug is relative to /docs
// ("" is the overview index itself). File is the .vane source's filename
// under src/pages/docs, used to build its "Edit this page" GitHub link.
type Topic struct {
	Slug     string
	Title    string
	Summary  string
	Category string
	File     string
}

// Manifest is the full docs table of contents, grouped by Category in
// display order.
var Manifest = []Topic{
	{Slug: "", Title: "Overview", Summary: "What Vane is and how the pieces fit together.", Category: "Getting Started", File: "Overview.vane"},
	{Slug: "concepts", Title: "Concepts", Summary: "The mental model: components, signals, no virtual DOM.", Category: "Getting Started", File: "Concepts.vane"},

	{Slug: "tutorial", Title: "Build a Todo List", Summary: "A hands-on walkthrough: signals, events, a reactive keyed list, and routing.", Category: "Tutorial", File: "Tutorial.vane"},
	{Slug: "tutorial-1-scaffold", Title: "Unit 1: Scaffold the Project", Summary: "Create the app, confirm the dev server works, add the tutorial's stylesheet.", Category: "Tutorial", File: "TutorialUnit1.vane"},
	{Slug: "tutorial-2-data-model", Title: "Unit 2: Model the Data & Render the List", Summary: "A signal-backed Todo store and a reactive, keyed list.", Category: "Tutorial", File: "TutorialUnit2.vane"},
	{Slug: "tutorial-3-add-todos", Title: "Unit 3: Add New Todos", Summary: "A controlled form input and a store function that appends to the list.", Category: "Tutorial", File: "TutorialUnit3.vane"},
	{Slug: "tutorial-4-toggle-remove", Title: "Unit 4: Toggle & Remove", Summary: "A checkbox and a remove button, each driving the store by item ID.", Category: "Tutorial", File: "TutorialUnit4.vane"},
	{Slug: "tutorial-5-filter-routes", Title: "Unit 5: Filter by Route", Summary: "Three routes, one page - the URL decides what's visible.", Category: "Tutorial", File: "TutorialUnit5.vane"},

	{Slug: "installation", Title: "Installation", Summary: "Install the CLI and scaffold a project with vane init.", Category: "Start a New Project", File: "Installation.vane"},
	{Slug: "vscode-extension", Title: "VS Code Extension", Summary: "Syntax highlighting and a real language server for .vane files, plus the workspace settings it manages for you.", Category: "Start a New Project", File: "VSCodeExtension.vane"},
	{Slug: "project-structure", Title: "Project Structure", Summary: "What vane init scaffolds: App.vane, src/pages, src/components, public/.", Category: "Start a New Project", File: "ProjectStructure.vane"},
	{Slug: "develop-and-build", Title: "Develop & Build", Summary: "The dev loop with vane run, production builds with vane build, deploying dist/.", Category: "Start a New Project", File: "DevelopAndBuild.vane"},
	{Slug: "signals", Title: "Signals & Reactivity", Summary: "Effect, OnDispose, ComputedOf, Untrack.", Category: "Start a New Project", File: "Signals.vane"},

	{Slug: "components", Title: "Components", Summary: "Functions, children, the controller pattern.", Category: "Build Your UI", File: "Components.vane"},
	{Slug: "vane-syntax", Title: "Vane Syntax", Summary: "Full prop/event table, inline for/if/switch.", Category: "Build Your UI", File: "VaneSyntax.vane"},
	{Slug: "lists", Title: "Lists", Summary: "The {for}+key pattern to reach for by default, {items()...} for raw nodes, and core.List[T] for field-level reactive collections.", Category: "Build Your UI", File: "Lists.vane"},
	{Slug: "style", Title: "Styles and CSS", Summary: "The core.Style struct and co-located CSS.", Category: "Build Your UI", File: "Style.vane"},

	{Slug: "routing", Title: "Routing", Summary: "Router, params, layouts, ActiveLink.", Category: "Data & Navigation", File: "Routing.vane"},
	{Slug: "store", Title: "Global Store", Summary: "Package-level signals for shared state.", Category: "Data & Navigation", File: "Store.vane"},
	{Slug: "refs-and-dom", Title: "Refs & DOM", Summary: "Reading and imperatively touching DOM nodes.", Category: "Data & Navigation", File: "RefsAndDOM.vane"},

	{Slug: "lifecycle", Title: "Lifecycle", Summary: "Mount, effect reruns, cleanup, and signal lifetime.", Category: "Advanced", File: "Lifecycle.vane"},
	{Slug: "portal", Title: "Portals", Summary: "Render into a DOM node outside the component tree.", Category: "Advanced", File: "Portal.vane"},
	{Slug: "head", Title: "Head Management", Summary: "Reactive document.title and meta tags.", Category: "Advanced", File: "Head.vane"},

	{Slug: "dos-and-donts", Title: "Do's and Don'ts", Summary: "Vane-specific conventions: where Vane syntax literals are allowed, Untrack for setup reads, and other easy mistakes.", Category: "Best Practices", File: "DosAndDonts.vane"},
	{Slug: "security", Title: "Security", Summary: "DangerousInnerHTML, escaping untrusted input, and other Vane security considerations.", Category: "Best Practices", File: "Security.vane"},
	{Slug: "accessibility", Title: "Accessibility", Summary: "aria-*/role, focus management, live regions.", Category: "Best Practices", File: "Accessibility.vane"},
	{Slug: "error-boundary", Title: "Handle Errors", Summary: "Catch panics from a subtree without crashing the app.", Category: "Best Practices", File: "HandleErrors.vane"},

	{Slug: "troubleshooting", Title: "Troubleshooting", Summary: "Known gotchas: reactive infinite loops, ref timing, and how to diagnose them.", Category: "Troubleshooting", File: "Troubleshooting.vane"},

	{Slug: "patterns", Title: "Patterns", Summary: "Patterns for common UI problems.", Category: "How-to Patterns", File: "Patterns.vane"},
	{Slug: "bundle-size", Title: "Analyze Bundle Size", Summary: "Why the wasm binary is large, and how to inspect and trim it.", Category: "How-to Patterns", File: "BundleSize.vane"},
	{Slug: "data-fetching", Title: "Data Fetching", Summary: "Fetch data with a goroutine and net/http, no async library needed.", Category: "How-to Patterns", File: "DataFetching.vane"},
	{Slug: "html-forms", Title: "Build HTML Forms", Summary: "Controlled inputs, validation, and submit handling in Vane.", Category: "How-to Patterns", File: "HTMLForms.vane"},
	{Slug: "lucide-icons", Title: "Add Lucide Icons", Summary: "Wire up the Lucide icon library, the same way this site's own Nav does.", Category: "How-to Patterns", File: "LucideIcons.vane"},

	{Slug: "cli-reference", Title: "CLI Reference", Summary: "Every vane command and flag: init, run, build, compile, lsp, version.", Category: "CLI", File: "CLIReference.vane"},
	{Slug: "api-reference", Title: "API Reference", Summary: "Curated index of Vane's public API by concept, linking to pkg.go.dev.", Category: "Reference", File: "APIReference.vane"},
	{Slug: "language-spec", Title: ".vane Language Spec", Summary: "The syntax .vane files must follow, and what's covered by Vane's compatibility contract.", Category: "Reference", File: "LanguageSpec.vane"},
	{Slug: "performance", Title: "Performance", Summary: "Vane performance measured against React, Svelte, and Solid.", Category: "Reference", File: "Performance.vane"},
}

// Sections is the top-level nav split shown above the sidebar: material
// split by kind, not by topic. Every Category maps to exactly one Section
// via categorySection below.
var Sections = []string{"Tutorial", "Guide", "CLI", "Reference"}

// categorySection maps each ad-hoc sidebar Category to its Section. Adding a
// Category to the manifest without an entry here is a bug - SectionOf falls
// back to "Guide" silently instead of panicking, so double check this map
// when a new Category shows up.
var categorySection = map[string]string{
	"Getting Started":     "Guide",
	"Tutorial":            "Tutorial",
	"Start a New Project": "Guide",
	"Build Your UI":       "Guide",
	"Data & Navigation":   "Guide",
	"Advanced":            "Guide",
	"Best Practices":      "Guide",
	"Troubleshooting":     "Guide",
	"How-to Patterns":     "Guide",
	"CLI":                 "CLI",
	"Reference":           "Reference",
}

func SectionOf(category string) string {
	if s, ok := categorySection[category]; ok {
		return s
	}
	return "Guide"
}

// CategoriesIn returns the Categories that belong to section, in the same
// relative order Categories() would list them.
func CategoriesIn(section string) []string {
	var out []string
	for _, c := range Categories() {
		if SectionOf(c) == section {
			out = append(out, c)
		}
	}
	return out
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
