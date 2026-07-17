package docs

// tocItem is one heading in a page's "on this page" list. ID must match the
// id set on the corresponding <h2>/<h3> in the same page. Plain Go (not a
// .vane file): search.go and every *TOCItems var below need this type at
// compile time even when no vane build has generated anything yet (gosec,
// go vet, gopls all type-check the raw checked-out tree), so it can't live
// only inside a generated _vane.go file the way it used to (widgets.vane).
type tocItem struct {
	ID    string
	Label string
}

var accessibilityTOCItems = []tocItem{
	{"aria-and-roles", "ARIA attributes and roles"},
	{"focus-management", "Focus management"},
	{"focus-trap", "Focus trap for modals"},
	{"accessible-modal-portal", "Accessible modal, with Portal"},
	{"live-regions", "Live regions"},
}

var componentsTOCItems = []tocItem{
	{"basic-components", "Basic components"},
	{"children", "Children"},
	{"htmlprops", "HTMLProps"},
	{"component-api", "Component API pattern"},
}

var conceptsTOCItems = []tocItem{
	{"vane-syntax", "Vane syntax"},
	{"compiler", "Vane is a compiler"},
	{"runtime", "Running in the browser"},
	{"lifecycle", "Component lifecycle"},
	{"rules", "Rules of thumb"},
}

var dataFetchingTOCItems = []tocItem{
	{"fetch-with-goroutine-and-signal", "Fetch with a goroutine and a signal"},
	{"loading-and-error-state", "Loading and error state"},
	{"why-no-asyncsignal", "Why there's no AsyncSignal"},
	{"cleanup", "What happens if the component unmounts before the fetch resolves"},
}

var developAndBuildTOCItems = []tocItem{
	{"dev-server", "Development server"},
	{"production-builds", "Production builds"},
	{"deploying", "Deploying dist/"},
	{"cli-reference", "CLI reference"},
}

var dosAndDontsTOCItems = []tocItem{
	{"vane-syntax-placement", "Where Vane syntax compiles"},
	{"signal-read-timing", "Read signals where they'll re-run"},
	{"untrack-setup-reads", "Untrack for manual component calls"},
}

var errorBoundaryTOCItems = []tocItem{
	{"basic-usage", "Basic usage"},
	{"what-it-catches", "What Try catches"},
	{"what-it-misses", "What Try doesn't catch"},
	{"effect-panics", "Effect panic recovery"},
}

var headTOCItems = []tocItem{
	{"basic-usage", "Basic usage"},
	{"reactive-updates", "Reactive updates"},
	{"scoping", "Scoping and router.Route's title"},
	{"field-reference", "Field reference"},
}

var htmlFormsTOCItems = []tocItem{
	{"controlled-inputs", "Controlled inputs"},
	{"skip-signals", "Skip signals when you don't need live values"},
	{"validation", "Validation"},
	{"handling-submit", "Handling submit"},
	{"reusable-fields", "Reusable field components"},
}

var installationTOCItems = []tocItem{
	{"prerequisites", "Prerequisites"},
	{"install-cli", "Install the CLI"},
	{"create-project", "Create a new project"},
	{"run-it", "Run it"},
}

var jsxSyntaxTOCItems = []tocItem{
	{"props-and-events", "Props & events"},
	{"jsx-rules", "JSX rules"},
	{"return-nil", "Early exits with return nil"},
	{"inline-control-flow", "Inline control flow"},
	{"reactive-lists", "Reactive lists"},
}

var lucideIconsTOCItems = []tocItem{
	{"load-the-script", "Load the script"},
	{"render-an-icon", "Render an icon"},
	{"icons-behind-a-binding", "Icons behind a reactive binding"},
}

var patternsTOCItems = []tocItem{
	{"fine-grained-signals", "Fine-grained signals"},
	{"reactive-lists", "Reactive lists and keys"},
	{"goroutines", "Goroutines and cleanup"},
	{"portal-boundary", "Portal's reactive boundary"},
	{"testing", "Testing signal logic"},
}

var portalTOCItems = []tocItem{
	{"why", "Why use a Portal?"},
	{"target", "Choosing a target"},
	{"guard", "Guarding inside the Portal"},
	{"first-render-timing", "First render timing"},
}

var projectStructureTOCItems = []tocItem{
	{"the-tree", "The scaffolded tree"},
	{"root-files", "Root files"},
	{"public", "public/"},
	{"src", "src/"},
	{"co-located-css", "Co-located CSS"},
}

var refsAndDomTOCItems = []tocItem{
	{"pointer-refs", "Pointer refs"},
	{"escape-hatch", "The Unwrap escape hatch"},
	{"browser-apis", "Browser API wrappers"},
	{"callback-refs", "Callback refs"},
	{"nexttick", "NextTick and the live-DOM problem"},
}

var routingTOCItems = []tocItem{
	{"basic-routes", "Basic routes"},
	{"url-params", "URL params"},
	{"layouts", "Layouts"},
	{"navigation", "Navigation"},
	{"active-link", "ActiveLink"},
	{"path-signal", "Path signal"},
}

var securityTOCItems = []tocItem{
	{"the-default", "The default: structurally safe"},
	{"dangerousinnerhtml", "DangerousInnerHTML"},
	{"blocked-innerhtml-prop", "The blocked innerHTML/outerHTML prop"},
	{"url-props", "URL props (href, src)"},
}

var signalsTOCItems = []tocItem{
	{"creating-signals", "Creating signals"},
	{"effect", "Effect"},
	{"ondispose", "OnDispose"},
	{"computedof", "ComputedOf"},
	{"untrack", "Untrack"},
}

var storeTOCItems = []tocItem{
	{"the-pattern", "The pattern"},
	{"reading-and-writing", "Reading and writing"},
	{"effects-in-the-store", "Effects in the store"},
	{"derived-state", "Derived state"},
	{"when-to-use-it", "When to use it"},
}

var styleTOCItems = []tocItem{
	{"the-style-struct", "The core.Style struct"},
	{"reactive-styles", "Reactive styles"},
	{"static-strings", "Static CSS strings"},
	{"co-located-css", "Co-located CSS"},
}

var troubleshootingTOCItems = []tocItem{
	{"infinite-loops", "Infinite reactive update loops"},
	{"ref-timing", "Ref timing"},
	{"silent-panics", "Silent effect panics"},
	{"portal-target", "Portal target not found"},
	{"list-keys", "DynList key warnings"},
}
