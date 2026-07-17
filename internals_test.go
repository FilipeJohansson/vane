package main

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filipejohansson/vane/internal/compiler"
	"github.com/filipejohansson/vane/internal/hotreload"
)

func TestReorderArgs_BooleanFlagAfterPositional(t *testing.T) {
	got := reorderArgs([]string{"./myapp", "--tinygo"})
	want := []string{"--tinygo", "./myapp"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReorderArgs_ValueFlagAfterPositional(t *testing.T) {
	// Regression: reorderArgs used to classify each token by its own "-"
	// prefix, so "8080" (the value for "--port") landed in the positional
	// bucket right along with the real positional arg, and the two swapped
	// places relative to "--port" instead of staying paired with it.
	got := reorderArgs([]string{"./myapp", "--port", "8080"})
	want := []string{"--port", "8080", "./myapp"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReorderArgs_ValueFlagAlreadyFirst(t *testing.T) {
	got := reorderArgs([]string{"--port", "8080", "./myapp"})
	want := []string{"--port", "8080", "./myapp"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExtractLineMappings_Empty(t *testing.T) {
	entries := extractLineMappings("package main\nfunc App() {}\n")
	if len(entries) != 0 {
		t.Errorf("want 0 entries, got %d", len(entries))
	}
}

func TestExtractLineMappings_SingleDirective(t *testing.T) {
	// i=1 (0-indexed), GoLine = i+2 = 3
	src := "package main\n//line App.vane:5\nfunc App() {}\n"
	entries := extractLineMappings(src)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].VaneLine != 5 {
		t.Errorf("VaneLine: want 5, got %d", entries[0].VaneLine)
	}
	if entries[0].GoLine != 3 {
		t.Errorf("GoLine: want 3, got %d", entries[0].GoLine)
	}
}

func TestExtractLineMappings_MultipleDirectives(t *testing.T) {
	lines := []string{
		"//go:build js && wasm",     // 0
		"",                          // 1
		"package main",              // 2
		"//line App.vane:1",         // 3 → GoLine = 3+2 = 5
		"func App() js.Value {",     // 4
		"//line App.vane:3",         // 5 → GoLine = 5+2 = 7
		`    return core.El("div")`, // 6
		"}",                         // 7
	}
	entries := extractLineMappings(strings.Join(lines, "\n"))
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d: %v", len(entries), entries)
	}
	if entries[0].VaneLine != 1 || entries[0].GoLine != 5 {
		t.Errorf("entry[0]: want {VaneLine:1, GoLine:5}, got %+v", entries[0])
	}
	if entries[1].VaneLine != 3 || entries[1].GoLine != 7 {
		t.Errorf("entry[1]: want {VaneLine:3, GoLine:7}, got %+v", entries[1])
	}
}

func TestExtractLineMappings_URLFilename(t *testing.T) {
	src := "package main\n//line http://localhost:8080/__vane_src/App.vane:42\nfunc App() {}\n"
	entries := extractLineMappings(src)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].VaneLine != 42 {
		t.Errorf("VaneLine: want 42, got %d", entries[0].VaneLine)
	}
}

func TestExtractLineMappings_SkipsNonDirectives(t *testing.T) {
	src := strings.Join([]string{
		"package main",
		"// line App.vane:1",   // space before "line", not a directive
		"//linefoo App.vane:2", // no space after //line
		"//line App.vane:3",    // real directive → GoLine = 2+2 = 4
		"func App() {}",
	}, "\n")
	entries := extractLineMappings(src)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry (only the real directive), got %d: %v", len(entries), entries)
	}
	if entries[0].VaneLine != 3 {
		t.Errorf("VaneLine: want 3, got %d", entries[0].VaneLine)
	}
}

// minimalVane is a .vane file that compiles without imports from vane.
// The compiler only touches vane syntax inside return statements and leaves
// imports verbatim, so these overlay tests don't need a real vane import.
const minimalVane = `package main

import "syscall/js"

func Hello() core.Node {
	return (<div>hello</div>)
}
`

func TestBuildOverlay_NoVaneFiles(t *testing.T) {
	tmp := t.TempDir()
	overlayPath, files, srcMap, cleanup, err := buildOverlay(tmp, "")
	if err != nil {
		t.Fatalf("buildOverlay: %v", err)
	}
	defer cleanup()
	if overlayPath != "" {
		t.Errorf("want empty overlayPath for empty project, got %q", overlayPath)
	}
	if len(files) != 0 {
		t.Errorf("want 0 compiled files, got %d", len(files))
	}
	if srcMap == nil {
		t.Error("srcMap must not be nil even for empty project")
	}
}

func TestBuildOverlay_SingleVaneFile(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "Hello.vane"), []byte(minimalVane), 0644)

	overlayPath, files, srcMap, cleanup, err := buildOverlay(tmp, "")
	if err != nil {
		t.Fatalf("buildOverlay: %v", err)
	}
	defer cleanup()

	if len(files) != 1 {
		t.Errorf("want 1 compiled file, got %d", len(files))
	}
	if overlayPath == "" {
		t.Fatal("want non-empty overlayPath")
	}
	if _, statErr := os.Stat(overlayPath); statErr != nil {
		t.Fatalf("overlay file not found: %v", statErr)
	}

	// Overlay JSON must have exactly one Replace entry.
	data, _ := os.ReadFile(overlayPath)
	var ol struct {
		Replace map[string]string `json:"Replace"`
	}
	if err := json.Unmarshal(data, &ol); err != nil {
		t.Fatalf("parsing overlay JSON: %v", err)
	}
	if len(ol.Replace) != 1 {
		t.Errorf("want 1 Replace entry, got %d: %v", len(ol.Replace), ol.Replace)
	}

	// Each Replace key must use the _vane.go suffix (same file the LSP writes).
	for key, val := range ol.Replace {
		if !strings.HasSuffix(key, "_vane.go") {
			t.Errorf("overlay key should end with _vane.go, got %q", key)
		}
		if _, err := os.Stat(val); err != nil {
			t.Errorf("overlay target file not found: %q → %v", val, err)
		}
	}

	if len(srcMap.Files) == 0 {
		t.Error("srcMap.Files must not be empty")
	}
}

func TestBuildOverlay_SkipsDistAndPublic(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "App.vane"), []byte(minimalVane), 0644)

	for _, skip := range []string{"dist", "public"} {
		dir := filepath.Join(tmp, skip)
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "Skip.vane"), []byte(minimalVane), 0644)
	}

	_, files, _, cleanup, err := buildOverlay(tmp, "")
	if err != nil {
		t.Fatalf("buildOverlay: %v", err)
	}
	defer cleanup()

	if len(files) != 1 {
		t.Errorf("want 1 compiled file (only App.vane), got %d", len(files))
	}
}

func TestBuildOverlay_SourceURLBase(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "App.vane"), []byte(minimalVane), 0644)

	const urlBase = "http://localhost:8080/__vane_src/"
	overlayPath, _, srcMap, cleanup, err := buildOverlay(tmp, urlBase)
	if err != nil {
		t.Fatalf("buildOverlay: %v", err)
	}
	defer cleanup()

	// Generated Go file must contain the URL base in a //line directive.
	data, _ := os.ReadFile(overlayPath)
	var ol struct {
		Replace map[string]string `json:"Replace"`
	}
	json.Unmarshal(data, &ol)
	for _, genPath := range ol.Replace {
		src, _ := os.ReadFile(genPath)
		if !strings.Contains(string(src), urlBase) {
			t.Errorf("generated file should contain %q in //line directive, got:\n%s", urlBase, src)
		}
		break
	}

	if len(srcMap.Files) == 0 {
		t.Error("srcMap.Files must not be empty in debug mode")
	}
}

func TestBuildOverlay_CleanupRemovesTempFiles(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "App.vane"), []byte(minimalVane), 0644)

	overlayPath, _, _, cleanup, err := buildOverlay(tmp, "")
	if err != nil {
		t.Fatalf("buildOverlay: %v", err)
	}
	if _, err := os.Stat(overlayPath); err != nil {
		t.Fatal("overlay must exist before cleanup")
	}
	cleanup()
	if _, err := os.Stat(overlayPath); !os.IsNotExist(err) {
		t.Error("overlay must be removed after cleanup")
	}
}

func TestBuildOverlay_DeduplicatesSameNamedFiles(t *testing.T) {
	tmp := t.TempDir()
	// Two files named "App.vane" in different subdirectories.
	os.MkdirAll(filepath.Join(tmp, "a"), 0755)
	os.MkdirAll(filepath.Join(tmp, "b"), 0755)
	os.WriteFile(filepath.Join(tmp, "a", "App.vane"), []byte(minimalVane), 0644)
	os.WriteFile(filepath.Join(tmp, "b", "App.vane"), []byte(minimalVane), 0644)

	overlayPath, files, _, cleanup, err := buildOverlay(tmp, "")
	if err != nil {
		t.Fatalf("buildOverlay: %v", err)
	}
	defer cleanup()

	if len(files) != 2 {
		t.Errorf("want 2 compiled files, got %d", len(files))
	}

	data, _ := os.ReadFile(overlayPath)
	var ol struct {
		Replace map[string]string `json:"Replace"`
	}
	json.Unmarshal(data, &ol)
	if len(ol.Replace) != 2 {
		t.Errorf("want 2 Replace entries (no collision), got %d: %v", len(ol.Replace), ol.Replace)
	}
}

func TestMainGoTemplateIsValidGo(t *testing.T) {
	src := mainGoTemplate()
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "", src, parser.AllErrors); err != nil {
		t.Fatalf("mainGoTemplate is not valid Go: %v\n%s", err, src)
	}
	if !strings.Contains(src, "//go:build js && wasm") {
		t.Error("mainGoTemplate missing build tag")
	}
	if !strings.Contains(src, "core.Mount") {
		t.Error("mainGoTemplate missing core.Mount call")
	}
}

func TestGoModTemplate(t *testing.T) {
	mod := goModTemplate("example.com/my-app")
	if !strings.Contains(mod, "module example.com/my-app") {
		t.Error("goModTemplate: missing module declaration")
	}
	if strings.Contains(mod, "replace") {
		t.Error("goModTemplate: should not contain a replace directive")
	}
	if strings.Contains(mod, "require") {
		t.Error("goModTemplate: require is left for `go mod tidy` to resolve")
	}
}

func TestGitignoreTemplateContainsDist(t *testing.T) {
	content := gitignoreTemplate()
	if !strings.Contains(content, "dist/") {
		t.Error("gitignoreTemplate: missing dist/")
	}
}

// TestIndexHTMLTemplateRewritesPathnameToHash guards against a direct load
// of a path other than "/" (a shared link, a typo, a bookmark) leaving a
// stale pathname in the address bar forever. The router is hash-based and
// only ever reads location.hash, so boot.js seeds the hash from the
// pathname on boot - via history.replaceState, which rewrites the visible
// URL, not a bare "location.hash = ..." assignment, which leaves the
// original pathname sitting in front of the hash for the rest of the
// session (e.g. clicking a link home would land on "/gsdg#/" instead of
// "/#/").
func TestIndexHTMLTemplateRewritesPathnameToHash(t *testing.T) {
	html := bootJSTemplate()
	if !strings.Contains(html, "history.replaceState") {
		t.Error("bootJSTemplate: expected a history.replaceState call to rewrite the pathname into the hash")
	}
	if strings.Contains(html, "location.hash = \"#\" + location.pathname") {
		t.Error("bootJSTemplate: found the old bare location.hash assignment, which leaves the original pathname in the URL")
	}
}

// TestIndexHTMLTemplateUsesExternalBootJS guards the CSP-friendliness fix:
// index.html must reference boot.js via a same-origin <script src>, not
// inline <script> blocks, so an app's CSP can use script-src 'self' instead
// of being forced into 'unsafe-inline'.
func TestIndexHTMLTemplateUsesExternalBootJS(t *testing.T) {
	html := indexHTMLTemplate("my-app")
	if !strings.Contains(html, `<script src="boot.js"></script>`) {
		t.Error("indexHTMLTemplate: expected a <script src=\"boot.js\"></script> reference")
	}
	if strings.Contains(html, "instantiateStreaming") {
		t.Error("indexHTMLTemplate: found inline WASM-boot code, expected it to live in boot.js instead")
	}
}

// TestBundleCSS_CollapsesImportsIntoOneFile guards the release-only CSS
// bundling fix: vane-page's own Lighthouse run measured 590-1930ms wasted
// on ~20 separate render-blocking requests for co-located Component.css
// files pulled in via @import. bundleCSS runs after copyCSSFiles has
// already flattened every import target into distDir, so it collapses
// them in place with no network involved.
func TestBundleCSS_CollapsesImportsIntoOneFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("style.css", "@import 'Button.css';\n@import 'Nav.css';\n\nbody { margin: 0; }\n")
	write("Button.css", ".btn { color: red; }")
	write("Nav.css", ".nav { color: blue; }")

	if err := bundleCSS(dir); err != nil {
		t.Fatalf("bundleCSS: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "style.css"))
	if err != nil {
		t.Fatal(err)
	}
	gotStr := string(got)
	if strings.Contains(gotStr, "@import") {
		t.Errorf("bundleCSS: expected @import lines stripped, got:\n%s", gotStr)
	}
	for _, want := range []string{".btn { color: red; }", ".nav { color: blue; }", "body { margin: 0; }"} {
		if !strings.Contains(gotStr, want) {
			t.Errorf("bundleCSS: expected output to contain %q, got:\n%s", want, gotStr)
		}
	}
	if strings.Index(gotStr, ".btn") > strings.Index(gotStr, "body { margin: 0; }") {
		t.Errorf("bundleCSS: expected imported files before the rest of style.css, got:\n%s", gotStr)
	}

	// The per-component files must be gone: once their contents live in
	// style.css, leaving Button.css/Nav.css behind in dist/ is dead weight
	// nothing links to, defeating the point of bundling.
	for _, name := range []string{"Button.css", "Nav.css"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("bundleCSS: expected %s removed from dist after inlining, stat err: %v", name, err)
		}
	}
}

// TestBundleCSS_NoImportsLeavesFileUnchanged covers vane init's scaffolded
// style.css, which has no @import lines at all (it's not co-located CSS,
// see styleCSSTemplate).
func TestBundleCSS_NoImportsLeavesFileUnchanged(t *testing.T) {
	dir := t.TempDir()
	const content = "body { margin: 0; }\n"
	if err := os.WriteFile(filepath.Join(dir, "style.css"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	if err := bundleCSS(dir); err != nil {
		t.Fatalf("bundleCSS: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "style.css"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("bundleCSS: expected file unchanged with no @import lines, got:\n%s", got)
	}
}

// TestBundleCSS_MissingStyleCSSIsNoOp covers projects with no public/style.css at all.
func TestBundleCSS_MissingStyleCSSIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := bundleCSS(dir); err != nil {
		t.Errorf("bundleCSS: expected no error for missing style.css, got: %v", err)
	}
}

func TestWasmBuildArgs_ReleaseStripsSymbolsAndDebugInfo(t *testing.T) {
	args := wasmBuildArgs("dist/app.wasm", "", true, false, "", ".")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-ldflags=-X github.com/filipejohansson/vane/core.releaseFlag=1 -s -w") {
		t.Errorf("release build args missing -s -w strip flags: %v", args)
	}
}

func TestBuildErrorHint_StyleStringMismatch(t *testing.T) {
	msg := "cannot use s (variable of type string) as core.Style value in return statement"
	hint := buildErrorHint(msg)
	if !strings.Contains(hint, "core.Style") || !strings.Contains(hint, `style="`) {
		t.Errorf("expected a hint pointing at style={core.Style{...}} and the style=\"...\" literal form, got: %q", hint)
	}
}

func TestBuildErrorHint_UnrecognizedMessageReturnsEmpty(t *testing.T) {
	if hint := buildErrorHint("undefined: foo"); hint != "" {
		t.Errorf("expected no hint for an unrelated go build error, got: %q", hint)
	}
}

func TestWasmBuildArgs_NonReleaseHasNoLdflags(t *testing.T) {
	args := wasmBuildArgs("dist/app.wasm", "", false, false, "", ".")
	for _, a := range args {
		if strings.HasPrefix(a, "-ldflags") {
			t.Errorf("non-release build should have no -ldflags, got: %v", args)
		}
	}
}

func TestWasmBuildArgs_DebugBuildOmitsStripFlags(t *testing.T) {
	// vane debug passes gcflags but release=false, so -s -w must never appear
	// here - DevTools relies on the unstripped name section for goroutine names.
	args := wasmBuildArgs("dist/app.wasm", "all=-N -l", false, false, "", ".")
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "-s -w") {
		t.Errorf("debug build args must not strip symbols/debug info: %v", args)
	}
}

func TestWasmBuildArgs_OverlayAndPkgPathIncluded(t *testing.T) {
	args := wasmBuildArgs("dist/app.wasm", "", false, false, "/tmp/overlay.json", "./src")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-overlay /tmp/overlay.json") {
		t.Errorf("expected -overlay flag with path, got: %v", args)
	}
	if args[len(args)-1] != "./src" {
		t.Errorf("expected pkgPath as last arg, got: %v", args)
	}
}

func TestWasmBuildArgs_ReleaseSkipOptimizeOmitsStripFlags(t *testing.T) {
	// --no-optimize must still keep the core.releaseFlag ldflag (dev warnings
	// stay stripped) but drop -s -w - it's an escape hatch for the size
	// optimizations specifically, not for release mode itself.
	args := wasmBuildArgs("dist/app.wasm", "", true, true, "", ".")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-ldflags=-X github.com/filipejohansson/vane/core.releaseFlag=1") {
		t.Errorf("release+skipOptimize build should still set core.releaseFlag: %v", args)
	}
	if strings.Contains(joined, "-s -w") {
		t.Errorf("release+skipOptimize build must not include -s -w: %v", args)
	}
}

func TestTinygoBuildArgs_ReleaseSetsReleaseFlagLdflag(t *testing.T) {
	// vane build --tinygo --release used to silently ignore --release: no
	// -ldflags was ever passed to `tinygo build`, so core.releaseFlag stayed
	// unset and core.Warn calls shipped in the "release" binary same as dev.
	args := tinygoBuildArgs("dist/app.wasm", false, true)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-ldflags=-X github.com/filipejohansson/vane/core.releaseFlag=1") {
		t.Errorf("tinygo release build args missing core.releaseFlag ldflag: %v", args)
	}
}

func TestTinygoBuildArgs_NonReleaseHasNoLdflags(t *testing.T) {
	args := tinygoBuildArgs("dist/app.wasm", false, false)
	for _, a := range args {
		if strings.HasPrefix(a, "-ldflags") {
			t.Errorf("non-release tinygo build should have no -ldflags, got: %v", args)
		}
	}
}

func TestTinygoBuildArgs_DebugBuildOmitsReleaseFlags(t *testing.T) {
	// cmdRun always passes release=false for a --debug --tinygo run, but this
	// also guards against debug+release ever being combined by mistake: debug
	// flags and the release ldflag should never appear together.
	args := tinygoBuildArgs("dist/app.wasm", true, false)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-no-debug=false") || !strings.Contains(joined, "-opt=1") {
		t.Errorf("debug tinygo build args missing debug flags: %v", args)
	}
	if strings.Contains(joined, "-ldflags") {
		t.Errorf("debug tinygo build args must not set core.releaseFlag: %v", args)
	}
}

func TestTinygoBuildArgs_DebugAndReleaseCoexist(t *testing.T) {
	args := tinygoBuildArgs("dist/app.wasm", true, true)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-no-debug=false") {
		t.Errorf("expected debug flags alongside release, got: %v", args)
	}
	if !strings.Contains(joined, "-ldflags=-X github.com/filipejohansson/vane/core.releaseFlag=1") {
		t.Errorf("expected core.releaseFlag ldflag alongside debug flags, got: %v", args)
	}
}

func TestTinygoBuildArgs_OutputAndPackagePathIncluded(t *testing.T) {
	args := tinygoBuildArgs("dist/app.wasm", false, false)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-o dist/app.wasm") {
		t.Errorf("expected -o flag with wasm output path, got: %v", args)
	}
	if args[len(args)-1] != "." {
		t.Errorf("expected \".\" package path as last arg, got: %v", args)
	}
}

func TestRunWasmOpt_MissingToolIsNotAnError(t *testing.T) {
	// Simulate wasm-opt not being on PATH by pointing PATH somewhere empty.
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	wasmPath := filepath.Join(t.TempDir(), "app.wasm")
	if err := os.WriteFile(wasmPath, []byte("not a real wasm file"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := runWasmOpt(wasmPath); err != nil {
		t.Errorf("runWasmOpt with no wasm-opt on PATH should silently no-op, got: %v", err)
	}
	// File must be untouched, since wasm-opt never ran.
	data, err := os.ReadFile(wasmPath) // #nosec G304 -- test-owned temp file
	if err != nil {
		t.Fatalf("reading wasmPath: %v", err)
	}
	if string(data) != "not a real wasm file" {
		t.Errorf("runWasmOpt modified the file despite wasm-opt being absent")
	}
}

func TestRunWasmOpt_InvalidInputReturnsError(t *testing.T) {
	if _, err := exec.LookPath("wasm-opt"); err != nil {
		t.Skip("wasm-opt not installed, skipping (optional dependency, see internal_docs/next-steps.md gap #11)")
	}
	wasmPath := filepath.Join(t.TempDir(), "app.wasm")
	if err := os.WriteFile(wasmPath, []byte("not a real wasm file"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	err := runWasmOpt(wasmPath)
	if err == nil {
		t.Fatal("expected runWasmOpt to fail on invalid wasm input, got nil")
	}
	if !strings.Contains(err.Error(), "--no-optimize") {
		t.Errorf("error should point at the --no-optimize escape hatch, got: %v", err)
	}
}

func TestParseRunArgs_DefaultPort(t *testing.T) {
	dir, port, debug, tinygo, err := parseRunArgs([]string{"./myapp"})
	if err != nil {
		t.Fatalf("parseRunArgs: %v", err)
	}
	if dir != "./myapp" || port != "8080" || debug || tinygo {
		t.Errorf("got dir=%q port=%q debug=%v tinygo=%v, want dir=\"./myapp\" port=\"8080\" debug=false tinygo=false", dir, port, debug, tinygo)
	}
}

func TestParseRunArgs_PortBeforeDir(t *testing.T) {
	dir, port, _, _, err := parseRunArgs([]string{"--port", "9090", "./myapp"})
	if err != nil {
		t.Fatalf("parseRunArgs: %v", err)
	}
	if dir != "./myapp" || port != "9090" {
		t.Errorf("got dir=%q port=%q, want dir=\"./myapp\" port=\"9090\"", dir, port)
	}
}

func TestParseRunArgs_PortAfterDir(t *testing.T) {
	// Regression: "vane run myapp --port 9090" used to silently keep the
	// default port, because flag.Parse stops consuming flags at the first
	// positional argument ("myapp") and never saw "--port" at all.
	dir, port, _, _, err := parseRunArgs([]string{"./myapp", "--port", "9090"})
	if err != nil {
		t.Fatalf("parseRunArgs: %v", err)
	}
	if dir != "./myapp" || port != "9090" {
		t.Errorf("got dir=%q port=%q, want dir=\"./myapp\" port=\"9090\"", dir, port)
	}
}

func TestParseRunArgs_NoDir(t *testing.T) {
	dir, port, _, _, err := parseRunArgs([]string{"--port", "9090"})
	if err != nil {
		t.Fatalf("parseRunArgs: %v", err)
	}
	if dir != "." || port != "9090" {
		t.Errorf("got dir=%q port=%q, want dir=\".\" port=\"9090\"", dir, port)
	}
}

func TestParseRunArgs_DebugFlag(t *testing.T) {
	dir, port, debug, tinygo, err := parseRunArgs([]string{"--debug", "./myapp"})
	if err != nil {
		t.Fatalf("parseRunArgs: %v", err)
	}
	if dir != "./myapp" || port != "8080" || !debug || tinygo {
		t.Errorf("got dir=%q port=%q debug=%v tinygo=%v, want dir=\"./myapp\" port=\"8080\" debug=true tinygo=false", dir, port, debug, tinygo)
	}
}

func TestParseRunArgs_DebugAndPortAfterDir(t *testing.T) {
	// Same reordering guarantee as TestParseRunArgs_PortAfterDir, now with
	// --debug thrown into the mix too.
	dir, port, debug, _, err := parseRunArgs([]string{"./myapp", "--port", "9090", "--debug"})
	if err != nil {
		t.Fatalf("parseRunArgs: %v", err)
	}
	if dir != "./myapp" || port != "9090" || !debug {
		t.Errorf("got dir=%q port=%q debug=%v, want dir=\"./myapp\" port=\"9090\" debug=true", dir, port, debug)
	}
}

func TestParseRunArgs_TinygoFlag(t *testing.T) {
	dir, port, debug, tinygo, err := parseRunArgs([]string{"--tinygo", "./myapp"})
	if err != nil {
		t.Fatalf("parseRunArgs: %v", err)
	}
	if dir != "./myapp" || port != "8080" || debug || !tinygo {
		t.Errorf("got dir=%q port=%q debug=%v tinygo=%v, want dir=\"./myapp\" port=\"8080\" debug=false tinygo=true", dir, port, debug, tinygo)
	}
}

func TestParseRunArgs_DebugAndTinygoCombine(t *testing.T) {
	dir, port, debug, tinygo, err := parseRunArgs([]string{"--debug", "--tinygo", "./myapp", "--port", "9090"})
	if err != nil {
		t.Fatalf("parseRunArgs: %v", err)
	}
	if dir != "./myapp" || port != "9090" || !debug || !tinygo {
		t.Errorf("got dir=%q port=%q debug=%v tinygo=%v, want dir=\"./myapp\" port=\"9090\" debug=true tinygo=true", dir, port, debug, tinygo)
	}
}

func TestDistFileExists(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "style.css"), []byte("body{}"), 0644)
	os.MkdirAll(filepath.Join(tmp, "sub"), 0755)

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"real file", "/style.css", true},
		{"missing file", "/nope.css", false},
		{"directory, not a file", "/sub", false},
		{"path traversal stays inside distDir", "/../../../../etc/passwd", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := distFileExists(tmp, tc.path); got != tc.want {
				t.Errorf("distFileExists(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// TestBuildDistMux_UnknownPathFallsBackToIndex is the regression test for
// the bug this session found live: a direct load of an unmapped path (the
// hash-router never sees the real route, so any bookmark/typo/shared link
// lands here) used to hit http.FileServer's bare "404 page not found" text
// instead of the app's own styled 404 page.
func TestBuildDistMux_UnknownPathFallsBackToIndex(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "index.html"), []byte("<html><body></body></html>"), 0644)

	mux := buildDistMux(tmp, "", nil)
	req := httptest.NewRequest(http.MethodGet, "/gsdg", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want 200 (index.html fallback, not a bare FileServer 404)", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("got Content-Type %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "<body>") {
		t.Errorf("body doesn't look like index.html: %s", rec.Body.String())
	}
}

func TestBuildDistMux_ServesRealFile(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "index.html"), []byte("<html></html>"), 0644)
	os.WriteFile(filepath.Join(tmp, "style.css"), []byte("body{color:red}"), 0644)

	mux := buildDistMux(tmp, "", nil)
	req := httptest.NewRequest(http.MethodGet, "/style.css", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if rec.Body.String() != "body{color:red}" {
		t.Errorf("got body %q, want the real file contents, not the index.html fallback", rec.Body.String())
	}
}

func TestBuildDistMux_RootInjectsHotReloadSnippet(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "index.html"), []byte("<html><body>hi</body></html>"), 0644)

	mux := buildDistMux(tmp, "", nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), hotreload.SSESnippet) {
		t.Error("index.html response is missing the injected hot-reload SSE snippet")
	}
}

func TestBuildDistMux_MissingIndexHTMLReturns404(t *testing.T) {
	tmp := t.TempDir() // empty: no index.html at all

	mux := buildDistMux(tmp, "", nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404 when dist/ has no index.html to fall back to", rec.Code)
	}
}

// TestVaneTemplatesCompile verifies that every .vane template produces valid Go
// when passed through the vane compiler.
func TestVaneTemplatesCompile(t *testing.T) {
	const mod = "example.com/testapp"

	cases := []struct {
		name string
		src  string
	}{
		{"appVaneTemplate", appVaneTemplate(mod)},
		{"homeVaneTemplate", homeVaneTemplate(mod)},
		{"aboutVaneTemplate", aboutVaneTemplate(mod)},
		{"navLinkVaneTemplate", navLinkVaneTemplate()},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := compiler.Compile(tc.src, tc.name+".vane")
			if err != nil {
				t.Fatalf("compiler.Compile failed: %v", err)
			}
			fset := token.NewFileSet()
			if _, parseErr := parser.ParseFile(fset, "", out, parser.AllErrors); parseErr != nil {
				t.Fatalf("compiled output is not valid Go: %v\noutput:\n%s", parseErr, out)
			}
		})
	}
}
