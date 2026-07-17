package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/adler32"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/filipejohansson/vane/internal/compiler"
	"github.com/filipejohansson/vane/internal/hotreload"
)

// ANSI color codes, disabled when NO_COLOR is set or terminal doesn't support them.
var (
	clReset  = "\033[0m"
	clBold   = "\033[1m"
	clDim    = "\033[2m"
	clGreen  = "\033[32m"
	clRed    = "\033[31m"
	clCyan   = "\033[36m"
	clYellow = "\033[33m"
	clVane   = "\033[96m" // bright cyan, Go's brand blue (#00ADD8)
)

func initColors() {
	off := os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"
	if !off && runtime.GOOS == "windows" {
		off = os.Getenv("WT_SESSION") == "" &&
			os.Getenv("ConEmuANSI") != "ON" &&
			os.Getenv("TERM_PROGRAM") == ""
	}
	if off {
		clReset = ""
		clBold = ""
		clDim = ""
		clGreen = ""
		clRed = ""
		clCyan = ""
		clYellow = ""
		clVane = ""
	}
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// startSpinner prints an animated, carriage-return-overwritten spinner with
// label until the returned stop func runs, so a slow step (go build, wasm-opt)
// that produces no output of its own doesn't look hung. Falls back to a
// single static line when ANSI escapes aren't supported (see initColors;
// clReset == "" is that same off signal), since \r-overwrite needs a real
// terminal.
func startSpinner(label string) func() {
	if clReset == "" {
		fmt.Printf("  %s...\n", label)
		return func() {}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			fmt.Printf("\r  %s%s%s %s", clCyan, spinnerFrames[i%len(spinnerFrames)], clReset, label)
			i++
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
	return func() {
		close(stop)
		<-done
		fmt.Print("\r\033[K") // clear the spinner line so the step's own ✓/✗ line starts clean
	}
}

func formatSize(n int64) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%d KB", n/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// vaneModuleVersion returns the resolved version of the running vane binary's
// own module (e.g. "v0.0.0-20260712120000-abcdef123456" for `go install
// .../vane@latest`), or "" if it was built without module version info
// (`go build`/`make`).
func vaneModuleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return ""
	}
	return info.Main.Version
}

func vaneVersion() string {
	if v := vaneModuleVersion(); v != "" {
		return " " + v
	}
	return ""
}

// errAlreadyReported is returned when an error has already been printed to
// stderr in a styled format. main() checks for it to suppress the redundant
// plain "error: ..." line.
var errAlreadyReported = errors.New("")

func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

type vaneSourceMap struct {
	Version int                        `json:"version"`
	Files   map[string][]vaneLineEntry `json:"files"`
}

type vaneLineEntry struct {
	VaneLine int `json:"vaneLine"`
	GoLine   int `json:"goLine"`
}

// extractLineMappings parses //line directives from generated Go source and
// returns the mapping from each .vane source line to its generated Go line.
func extractLineMappings(generatedSrc string) []vaneLineEntry {
	var entries []vaneLineEntry
	lines := strings.Split(generatedSrc, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "//line ") {
			continue
		}
		directive := strings.TrimPrefix(trimmed, "//line ")
		lastColon := strings.LastIndex(directive, ":")
		if lastColon < 0 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(directive[lastColon+1:]))
		if err != nil {
			continue
		}
		entries = append(entries, vaneLineEntry{VaneLine: n, GoLine: i + 2})
	}
	return entries
}

func main() {
	initColors()
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "build":
		buildFS := flag.NewFlagSet("build", flag.ContinueOnError)
		buildTinyGo := buildFS.Bool("tinygo", false, "build with TinyGo instead of Go")
		buildRelease := buildFS.Bool("release", false, "strip core.Warn developer warnings (production build)")
		buildNoOptimize := buildFS.Bool("no-optimize", false, "skip -s -w and wasm-opt size optimizations (--release only)")
		if err := buildFS.Parse(reorderArgs(os.Args[2:])); err != nil {
			os.Exit(1)
		}
		dir := "."
		if buildFS.NArg() > 0 {
			dir = buildFS.Arg(0)
		}
		if *buildTinyGo {
			err = cmdBuildTinyGo(dir, *buildRelease, *buildNoOptimize)
		} else {
			err = cmdBuild(dir, *buildRelease, *buildNoOptimize)
		}
	case "run":
		dir, port, debug, tinygo, perr := parseRunArgs(os.Args[2:])
		if perr != nil {
			os.Exit(1)
		}
		err = cmdRun(dir, port, debug, tinygo)
	case "init":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: vane init <module>")
			os.Exit(1)
		}
		err = cmdInit(os.Args[2])
	case "compile":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: vane compile <file.vane>")
			os.Exit(1)
		}
		err = cmdCompile(os.Args[2])
	// case "lsp":
	// 	err = lsp.Serve(os.Stdin, os.Stdout)
	case "version", "--version", "-v":
		v := vaneVersion()
		if v == "" {
			v = " (devel)"
		}
		fmt.Printf("vane%s\n", v)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		if err != errAlreadyReported {
			fmt.Fprintf(os.Stderr, "\n  %s✗%s %s\n\n", clRed, clReset, err)
		}
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("usage: vane <command> [args]")
	fmt.Println()
	fmt.Println("commands:")
	fmt.Println("  init    <module>                               scaffold a vane project in the current directory")
	fmt.Println("  run     [dir] [--port N] [--debug] [--tinygo]  build then serve dist/ on :8080 with hot-reload")
	fmt.Println("  build   [dir] [--tinygo] [--release]           compile .vane files and build WASM to dist/ (default: .)")
	fmt.Println("  compile <file>                                 compile a single .vane file to stdout (dry-run)")
	fmt.Println("  version                                        print the vane version")
	fmt.Println("  help                                           show this help")
	fmt.Println()
	fmt.Println("flags:")
	fmt.Println("  --debug        build without inlining/optimizations for accurate Go call stacks and")
	fmt.Println("                 readable goroutine names (vane run only; off by default, costs runtime speed)")
	fmt.Println("  --tinygo       use TinyGo compiler (smaller WASM, source files visible in DevTools)")
	fmt.Println("                 requires TinyGo + binaryen: https://tinygo.org/getting-started/install/")
	fmt.Println("  --release      strip core.Warn developer misuse warnings from the build (production)")
	fmt.Println("  --no-optimize  skip -s -w and wasm-opt size optimizations applied by --release")
	fmt.Println("                 (escape hatch if an optimization pass breaks your build)")
}

// cmdCompile compiles one .vane file and prints the result, useful for debugging.
func cmdCompile(path string) error {
	if !strings.HasSuffix(path, ".vane") {
		return fmt.Errorf("%s: not a .vane file", path)
	}
	src, err := os.ReadFile(path) // #nosec G304 G703 -- path is the file the developer named on the CLI (`vane compile <file>`), not attacker input
	if err != nil {
		return err
	}
	out, err := compiler.Compile(string(src), filepath.Base(path))
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// cmdBuild compiles .vane files via overlay, builds WASM, copies public/ and
// co-located .css files to dist/.
func cmdBuild(dir string, release, skipOptimize bool) error {
	return buildWasm(dir, "", "", false, release, skipOptimize)
}

// wasmBuildArgs builds the "go build" argument list for compiling app.wasm.
// release adds "-s -w" alongside the existing core.releaseFlag ldflag (unless
// skipOptimize is set): it strips the symbol table and debug info, a standard
// Go/WASM size reduction with no runtime behavior change (Go's own
// panic/stack-trace machinery uses pclntab, not the stripped symbol table).
// Only applied for release builds: vane run --debug deliberately keeps this
// data for readable goroutine names in DevTools (see cmdRun).
func wasmBuildArgs(wasmOut, gcflags string, release, skipOptimize bool, overlayPath, pkgPath string) []string {
	args := []string{"build", "-o", wasmOut}
	if gcflags != "" {
		args = append(args, "-gcflags="+gcflags)
	}
	if release {
		ldflags := "-ldflags=-X github.com/filipejohansson/vane/core.releaseFlag=1"
		if !skipOptimize {
			ldflags += " -s -w"
		}
		args = append(args, ldflags)
	}
	if overlayPath != "" {
		args = append(args, "-overlay", overlayPath)
	}
	args = append(args, pkgPath)
	return args
}

// runWasmOpt runs Binaryen's wasm-opt -Oz on the built binary, if the tool is
// present on PATH. --enable-bulk-memory is required: Go's js/wasm output uses
// memory.copy/memory.fill, which wasm-opt's default (MVP) feature set rejects
// outright. Optional dependency, same pattern as tinygo.go's tinygo binary:
// silently skipped when not installed, since most users won't have Binaryen.
// If wasm-opt IS installed but fails on the input, that's surfaced as a real
// build error rather than silently shipping an unoptimized binary, the
// escape hatch for that case is --no-optimize, not a silent fallback.
func runWasmOpt(wasmPath string) error {
	if _, err := exec.LookPath("wasm-opt"); err != nil {
		return nil
	}
	tmp := wasmPath + ".opt"
	cmd := exec.Command("wasm-opt", "-Oz", "--enable-bulk-memory", wasmPath, "-o", tmp) // #nosec G204 G702 -- fixed flags; wasmPath is vane's own build output, not attacker input
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("wasm-opt: %w\n%s\n(skip with --no-optimize)", err, stderrBuf.String())
	}
	return os.Rename(tmp, wasmPath)
}

// sourceURLBase: when non-empty, //line directives use this URL prefix instead
// of the absolute file path (enables Chrome DevTools to fetch sources over HTTP).
// rebuild: when true, uses "rebuilt" in the output line instead of "built".
// release: when true, strips core.Warn developer warnings from the build
// (via -ldflags -X, checked once at runtime)
// skipOptimize: when true, skips the -s -w ldflags and wasm-opt pass that
// release builds otherwise apply, an escape hatch for when either breaks a
// build, not something meant to be reached for daily use.
func buildWasm(dir, gcflags, sourceURLBase string, rebuild, release, skipOptimize bool) error {
	buildStart := time.Now()

	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		return fmt.Errorf("%s is a file, but vane build expects a project directory\n  to build the project:     vane build <dir>", filepath.Base(abs))
	}

	// Find go.mod before walking the tree for .vane files, so running from
	// outside any project fails immediately instead of after compiling everything it finds.
	modRoot, err := findModuleRoot(abs)
	if err != nil {
		return err
	}

	// Locate main.go to identify the main package directory.
	// Check src/ first for backwards-compat, then fall back to project root.
	mainPkgDir := abs
	for _, candidate := range []string{"src", "."} {
		p := filepath.Join(abs, candidate, "main.go")
		if candidate == "." {
			p = filepath.Join(abs, "main.go")
		}
		if _, err := os.Stat(p); err == nil {
			if candidate == "." {
				mainPkgDir = abs
			} else {
				mainPkgDir = filepath.Join(abs, candidate)
			}
			break
		}
	}

	// Walk entire project for .vane files (skips dist/ and public/).
	overlayPath, files, srcMap, cleanup, err := buildOverlay(abs, sourceURLBase)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  %s✗%s compile error\n\n", clRed, clReset)
		var pe *compiler.ParseError
		if errors.As(err, &pe) {
			fmt.Fprintln(os.Stderr, pe.Format(clGreen != ""))
		} else {
			fmt.Fprintf(os.Stderr, "  %s%s%s\n\n", clRed, err, clReset)
		}
		return errAlreadyReported
	}
	defer cleanup()

	if len(files) > 0 {
		fmt.Printf("  %s✓%s %-9s  %s\n", clGreen, clReset, "compiled", strings.Join(files, ", "))
	}

	relSrc, err := filepath.Rel(modRoot, mainPkgDir)
	if err != nil {
		return err
	}
	pkgPath := "."
	if relSrc != "." {
		pkgPath = "./" + filepath.ToSlash(relSrc)
	}

	distDir := filepath.Join(abs, "dist")
	// Only wipe on a fresh build, not every hot-reload rebuild: dist/ is being
	// served live during `vane run` (buildDistMux reads straight from disk),
	// so clearing it on each incremental rebuild would 404 in-flight requests
	// for no reason. A fresh build's dist/ can otherwise carry stale files
	// forever, e.g. a __vane_map.json a previous --debug run wrote never gets
	// deleted by a later plain/--release build, since nothing ever removed it.
	if !rebuild {
		if err := os.RemoveAll(distDir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(distDir, 0750); err != nil {
		return err
	}
	wasmOut := filepath.Join(distDir, "app.wasm")

	args := wasmBuildArgs(wasmOut, gcflags, release, skipOptimize, overlayPath, pkgPath)

	var stderrBuf bytes.Buffer
	cmd := exec.Command("go", args...) // #nosec G204 -- args are vane's own build flags (overlay path, package path), not attacker input; go binary is hardcoded
	cmd.Dir = modRoot
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "GOWORK=off")
	cmd.Stdout = io.Discard // a spinner owns this line now; go build has nothing worth streaming on success
	cmd.Stderr = &stderrBuf
	stopSpinner := startSpinner("building")
	err = cmd.Run()
	stopSpinner()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  %s✗%s build failed\n\n", clRed, clReset)
		printBuildDiagnostics(stderrBuf.String())
		return errAlreadyReported
	}

	if release && !skipOptimize {
		stopOptSpinner := startSpinner("optimizing (wasm-opt)")
		optErr := runWasmOpt(wasmOut)
		stopOptSpinner()
		if optErr != nil {
			fmt.Fprintf(os.Stderr, "\n  %s✗%s wasm-opt failed\n\n", clRed, clReset)
			return fmt.Errorf("%w", optErr)
		}
	}

	// Write source map for debug builds so the browser can resolve .vane lines.
	if sourceURLBase != "" && srcMap != nil && len(srcMap.Files) > 0 {
		if smData, err := json.MarshalIndent(srcMap, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(distDir, "__vane_map.json"), smData, 0o600)
		}
	}

	// Copy public/ → dist/ (index.html, wasm_exec.js, global CSS, images, …)
	publicDir := filepath.Join(abs, "public")
	if _, err := os.Stat(publicDir); err == nil {
		if err := copyDir(publicDir, distDir); err != nil {
			return fmt.Errorf("copy public: %w", err)
		}
	}

	// Copy co-located .css files from source dirs → dist/ (React-like pattern).
	if err := copyCSSFiles(abs, distDir); err != nil {
		return fmt.Errorf("copy css: %w", err)
	}

	// Release only: collapse style.css's @import chain into one file, so
	// production doesn't ship one render-blocking request per co-located
	// Component.css. Dev keeps files separate for hot-reload.
	if release {
		if err := bundleCSS(distDir); err != nil {
			return fmt.Errorf("bundle css: %w", err)
		}
	}

	sizeStr := ""
	if info, statErr := os.Stat(wasmOut); statErr == nil {
		sizeStr = fmt.Sprintf(" %s(%s)%s", clDim, formatSize(info.Size()), clReset)
	}
	verb := "built"
	if rebuild {
		verb = "rebuilt"
	}
	fmt.Printf("  %s✓%s %-9s  app.wasm%s %sin %.1fs%s\n",
		clGreen, clReset, verb, sizeStr, clDim, time.Since(buildStart).Seconds(), clReset)

	return nil
}

// parseRunArgs parses `vane run [dir] [--port N] [--debug] [--tinygo]`,
// tolerating either argument order via reorderArgs (flag.Parse alone stops
// consuming flags at the first positional argument, so
// "vane run myapp --port 9090" would otherwise silently keep the default port).
func parseRunArgs(args []string) (dir, port string, debug, tinygo bool, err error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	p := fs.String("port", "8080", "port to serve on")
	d := fs.Bool("debug", false, "build without inlining/optimizations for accurate Go call stacks")
	tg := fs.Bool("tinygo", false, "build with TinyGo instead of Go")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return "", "", false, false, err
	}
	dir = "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}
	return dir, *p, *d, *tg, nil
}

// cmdRun builds, serves dist/ on the given port, and hot-reloads on source
// changes. Always builds in dev mode (release=false), so runtime misuse
// warnings stay on.
//
// /__vane_src/ source serving and grouped panic output (VaneDebugSnippet)
// are always on: neither has a runtime cost, so there's no reason to gate
// them behind a flag. The __vane_map.json line map is too, for the standard
// Go build; buildWasmTinyGo doesn't generate one (TinyGo's own //line
// directives always use absolute file paths, not sourceURLBase, so there's
// nothing for it to write there yet).
//
// debug additionally builds without inlining/optimizations: -N -l for the
// standard build (accurate Go call stacks, readable goroutine names in
// DevTools), or TinyGo's own -no-debug=false -opt=1 (DWARF emission) when
// combined with tinygo. Off by default, unlike the rest of this list,
// because it isn't free - vane's reactivity model calls Signal.Get/Set on
// every {expr} re-run, and losing inlining on that path is something a dev
// actually feels, not just a theoretical cost.
//
// tinygo builds with TinyGo instead of the standard compiler (see
// buildWasmTinyGo), same tradeoffs as vane build --tinygo: smaller/faster
// binaries, but TinyGo's stdlib port isn't 100% compatible with standard Go
// (net/http in particular has known js/wasm gaps).
func cmdRun(dir, port string, debug, tinygo bool) error {
	sourceURLBase := "http://localhost:" + port + "/__vane_src/"

	fmt.Printf("\n  %s%svane%s%s\n\n", clVane, clBold, clReset, clDim+vaneVersion()+clReset)

	gcflags := ""
	if debug {
		gcflags = "all=-N -l"
	}

	var buildErr error
	if tinygo {
		buildErr = buildWasmTinyGo(dir, debug, sourceURLBase, false, false)
	} else {
		buildErr = buildWasm(dir, gcflags, sourceURLBase, false, false, false)
	}
	if buildErr != nil {
		return buildErr
	}

	if debug {
		if tinygo {
			fmt.Printf("  %sTinyGo emits DWARF, so source files are visible in Chrome DevTools Sources%s\n", clDim, clReset)
			fmt.Printf("  %s  install extension: C/C++ DevTools Support (DWARF)%s\n", clDim, clReset)
			fmt.Printf("  %s  note: breakpoints not supported (TinyGo WASM DWARF format limitation)%s\n\n", clDim, clReset)
		} else {
			fmt.Printf("  %sDevTools → call stacks show Go function names%s\n", clDim, clReset)
			fmt.Printf("  %sGo panics are grouped in the console via VaneDebugSnippet%s\n", clDim, clReset)
			fmt.Printf("\n  %snote: Go 1.24 js/wasm does not emit DWARF, so source-level breakpoints%s\n", clDim, clReset)
			fmt.Printf("  %s      are not yet possible. This will work automatically once Go%s\n", clDim, clReset)
			fmt.Printf("  %s      supports DWARF output for the js/wasm target.%s\n\n", clDim, clReset)
		}
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	return cmdServe(abs, abs, port, true, gcflags, sourceURLBase, debug, tinygo)
}

// cmdServe serves dist/ on the given port with hot-reload. Assumes dist/ is
// already built. srcDir: when non-empty, also serves that directory at
// /__vane_src/ for source lookup. showBanner: when true, prints the
// Local/Network URLs and "watching" line (vane run mode).
//
// gcflags/sourceURLBase/debug/tinygo are threaded through to the watcher's
// rebuild-on-change path too, so a --debug and/or --tinygo run keeps
// rebuilding the same way after the first file change, not just on the
// initial build. Previously this was hardcoded to always call the standard
// go build with no flags on every rebuild, regardless of how the project
// was first built - a --debug run silently lost its unoptimized build and
// source map, and a --tinygo run silently switched back to the standard Go
// compiler, both after the very first edit.
func cmdServe(dir, srcDir, port string, showBanner bool, gcflags, sourceURLBase string, debug, tinygo bool) error {
	abs, _ := filepath.Abs(dir)
	distDir := filepath.Join(abs, "dist")
	_ = mime.AddExtensionType(".wasm", "application/wasm")

	build := func(projectDir string, rebuild bool) error {
		if tinygo {
			return buildWasmTinyGo(projectDir, debug, sourceURLBase, false, false)
		}
		return buildWasm(projectDir, gcflags, sourceURLBase, rebuild, false, false)
	}

	hub := hotreload.NewHub()
	go hotreload.Watch(abs, distDir, hub, hotreload.Callbacks{
		Build: func(projectDir string) error { return build(projectDir, false) },
		Rebuild: func(projectDir string, changedFiles []string) error {
			fmt.Printf("\n  %s↻%s  %s\n", clCyan, clReset, strings.Join(changedFiles, ", "))
			return build(projectDir, true)
		},
		OnCSSChange: func(changedFiles []string) {
			fmt.Printf("\n  %s↻%s  %s\n", clCyan, clReset, strings.Join(changedFiles, ", "))
		},
		OnPublicChange: func() {
			fmt.Printf("\n  %s↻%s  public/\n", clCyan, clReset)
		},
		CopyDir:      copyDir,
		CopyCSSFiles: copyCSSFiles,
	})

	if showBanner {
		fmt.Printf("  %s➜%s  Local:    %shttp://localhost:%s/%s\n", clCyan, clReset, clCyan, port, clReset)
		if ip := getOutboundIP(); ip != "" {
			fmt.Printf("  %s➜%s  Network:  %shttp://%s:%s/%s\n", clCyan, clReset, clCyan, ip, port, clReset)
		}
		fmt.Printf("\n  %swatching for changes...%s\n\n", clDim, clReset)
	} else {
		fmt.Printf("serving %s on http://localhost:%s\n", distDir, port)
	}

	mux := buildDistMux(distDir, srcDir, hub)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServe()
}

// buildDistMux returns the handler that serves distDir, the compiled
// project's output directory. srcDir, when non-empty, additionally serves
// that directory at /__vane_src/ for source lookup and enables grouped
// panic output. hub wires in the hot-reload SSE endpoint; pass nil to omit
// it (e.g. in tests, where no watcher is running).
//
// Any request path that isn't "/", "/index.html", or a real file in
// distDir falls back to index.html. The router is hash-based (see
// core/router): the server never sees the actual route, since the
// fragment after '#' isn't sent over HTTP, it only sees whatever real
// path the browser was given directly (a shared link, a typo, a
// bookmark). Without this fallback, that request hits http.FileServer's
// bare "404 page not found" text instead of the app's own styled 404.
func buildDistMux(distDir, srcDir string, hub *hotreload.Hub) *http.ServeMux {
	fileHandler := http.FileServer(http.Dir(distDir))
	mux := http.NewServeMux()

	serveIndex := func(w http.ResponseWriter) {
		content, err := os.ReadFile(filepath.Join(distDir, "index.html")) // #nosec G304 -- "index.html" is a hardcoded literal, not r.URL.Path; dynamic paths below go through http.FileServer(http.Dir), which is traversal-safe
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		inject := hotreload.SSESnippet
		if srcDir != "" {
			inject += "\n" + hotreload.VaneDebugSnippet
		}
		html := strings.Replace(string(content), "</body>", inject+"\n</body>", 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html)) // #nosec G705 -- html is the project's own dist/index.html plus fixed hotreload snippets, never derived from r.URL.Path or any other request data
	}

	if hub != nil {
		mux.Handle("/__vane_reload", hotreload.SSEHandler(hub))
	}
	if srcDir != "" {
		mux.Handle("/__vane_src/", http.StripPrefix("/__vane_src/", http.FileServer(http.Dir(srcDir))))
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			serveIndex(w)
			return
		}
		if !distFileExists(distDir, r.URL.Path) {
			serveIndex(w)
			return
		}
		fileHandler.ServeHTTP(w, r)
	})
	return mux
}

// distFileExists reports whether urlPath resolves to a real, non-directory
// file under distDir. filepath.Clean collapses any ".." segments before the
// join, so a request can't escape distDir.
func distFileExists(distDir, urlPath string) bool {
	full := filepath.Join(distDir, filepath.Clean("/"+urlPath))
	info, err := os.Stat(full)
	return err == nil && !info.IsDir()
}

// adler32Path returns a short checksum of a file path string, used to
// disambiguate same-named .vane files from different subdirectories.
func adler32Path(path string) uint32 {
	return adler32.Checksum([]byte(path))
}

// buildOverlay compiles all .vane files under projectDir (skipping dist/ and public/)
// and returns:
//   - path to a temporary overlay JSON file (empty string if no .vane files found)
//   - number of .vane files compiled
//   - cleanup function that deletes all temp files
//
// sourceURLBase: when non-empty, //line directives use this URL + relative path
// instead of the absolute file path (e.g. "http://localhost:8080/__vane_src/").
func buildOverlay(projectDir, sourceURLBase string) (overlayPath string, files []string, srcMap *vaneSourceMap, cleanup func(), err error) {
	type overlayJSON struct {
		Replace map[string]string `json:"Replace"`
	}
	ol := overlayJSON{Replace: make(map[string]string)}
	srcMap = &vaneSourceMap{Version: 1, Files: make(map[string][]vaneLineEntry)}

	tmpDir, err := os.MkdirTemp("", "vane-*")
	if err != nil {
		return "", nil, nil, nil, err
	}
	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	cwd, _ := os.Getwd()

	skipDirs := map[string]bool{"dist": true, "public": true}

	walkErr := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".vane") {
			return nil
		}

		src, err := os.ReadFile(path) // #nosec G304 G122 -- path is discovered by walking the developer's own project dir, not attacker input
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(projectDir, path)
		relKey := filepath.ToSlash(rel)
		lineFilename := filepath.ToSlash(path)
		if sourceURLBase != "" {
			lineFilename = sourceURLBase + relKey
		} else if cwd != "" {
			if cwdRel, err := filepath.Rel(cwd, path); err == nil && !strings.HasPrefix(cwdRel, "..") {
				lineFilename = filepath.ToSlash(cwdRel)
			}
		}
		goSrc, err := compiler.Compile(string(src), lineFilename)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}

		srcMap.Files[relKey] = extractLineMappings(goSrc)

		// Write generated Go source into the shared temp dir.
		// Use a hash of the original path to avoid collisions between same-named
		// files in different subdirectories (e.g. a/Modal.vane and b/Modal.vane).
		baseName := fmt.Sprintf("%x_%s", adler32Path(path), strings.TrimSuffix(filepath.Base(path), ".vane")+"_vane.go")
		tfPath := filepath.Join(tmpDir, baseName)
		if err := os.WriteFile(tfPath, []byte(goSrc), 0o600); err != nil { // #nosec G703 -- tfPath is os.MkdirTemp's own dir + a hashed name, not attacker input
			return err
		}

		// Overlay key uses _vane.go, the same file the LSP writes to disk.
		// The overlay wins over whatever is on disk, so go build always uses
		// the compiled-with-//line version regardless of LSP concurrent writes.
		goPath := strings.TrimSuffix(path, ".vane") + "_vane.go"
		ol.Replace[goPath] = tfPath

		files = append(files, filepath.Base(path))
		return nil
	})
	if walkErr != nil {
		cleanup()
		return "", nil, nil, nil, walkErr
	}

	if len(files) == 0 {
		return "", files, srcMap, cleanup, nil
	}

	// Write overlay JSON into the same temp dir.
	data, err := json.Marshal(ol)
	if err != nil {
		cleanup()
		return "", nil, nil, nil, err
	}
	ofPath := filepath.Join(tmpDir, "overlay.json")
	if err := os.WriteFile(ofPath, data, 0o600); err != nil {
		cleanup()
		return "", nil, nil, nil, err
	}
	return ofPath, files, srcMap, cleanup, nil
}

// cmdInit scaffolds a vane project in the current directory, which must be
// empty (dotfiles like an existing .git are fine). module becomes the
// project's go.mod module path, mirroring `go mod init <module>`: mkdir and
// cd are the developer's job, not this command's.
func cmdInit(module string) error {
	entries, err := os.ReadDir(".")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		return fmt.Errorf("current directory is not empty, run vane init in a fresh directory")
	}

	for _, dir := range []string{
		"public",
		filepath.Join("src", "pages"),
		filepath.Join("src", "components"),
		filepath.Join("src", "store"),
	} {
		if err := os.MkdirAll(dir, 0750); err != nil { // #nosec G703 -- dir is a fixed scaffold path under the current (already-validated-empty) directory
			return err
		}
	}

	files := map[string]string{
		"go.mod":                                           goModTemplate(module),
		".gitignore":                                       gitignoreTemplate(),
		"main.go":                                          mainGoTemplate(),
		"App.vane":                                         appVaneTemplate(module),
		filepath.Join("public", "index.html"):              indexHTMLTemplate(filepath.Base(module)),
		filepath.Join("public", "boot.js"):                 bootJSTemplate(),
		filepath.Join("public", "style.css"):               styleCSSTemplate(),
		filepath.Join("public", "favicon.svg"):             faviconSVGTemplate(),
		filepath.Join("src", "pages", "Home.vane"):         homeVaneTemplate(module),
		filepath.Join("src", "pages", "About.vane"):        aboutVaneTemplate(module),
		filepath.Join("src", "components", "NavLink.vane"): navLinkVaneTemplate(),
		filepath.Join("src", "store", "user.go"):           storeUserGoTemplate(),
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			return err
		}
	}

	// Copy wasm_exec.js into public/ so it gets bundled into dist/ on build.
	if err := copyWasmExec("public"); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not copy wasm_exec.js: %v\n", err)
		fmt.Fprintln(os.Stderr, "  copy it manually: $(go env GOROOT)/lib/wasm/wasm_exec.js → public/")
	}

	// Resolve dependencies.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Stdout = os.Stdout
	tidy.Stderr = os.Stderr
	if err := tidy.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: go mod tidy failed: %v\n", err)
	}

	fmt.Printf("\n  %s%s✓%s%s initialized %s%s%s%s\n", clBold, clGreen, clReset, clDim, clReset, clBold, module, clReset)
	fmt.Printf("\n  %sget started:%s\n", clDim, clReset)
	fmt.Printf("  %svane run .%s\n\n", clDim, clReset)
	return nil
}

// copyWasmExec copies wasm_exec.js from GOROOT into destDir.
func copyWasmExec(destDir string) error {
	cmd := exec.Command("go", "env", "GOROOT")
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	goroot := strings.TrimSpace(string(out))
	// Go ≥1.21 moved the file to lib/wasm/; older versions had it in misc/wasm/.
	candidates := []string{
		filepath.Join(goroot, "lib", "wasm", "wasm_exec.js"),
		filepath.Join(goroot, "misc", "wasm", "wasm_exec.js"),
	}
	for _, src := range candidates {
		data, err := os.ReadFile(src) // #nosec G304 -- src is derived from `go env GOROOT`'s own output, not attacker input
		if err != nil {
			continue
		}
		return os.WriteFile(filepath.Join(destDir, "wasm_exec.js"), data, 0600) // #nosec G703 -- destDir is the project's own dist/ directory
	}
	return fmt.Errorf("wasm_exec.js not found in GOROOT (%s)", goroot)
}

// copyDir recursively copies all files from src into dst, creating dirs as needed.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0750) // #nosec G703 -- target is derived from walking the project's own src dir, not attacker input
		}
		data, err := os.ReadFile(path) // #nosec G304 G122 -- path is discovered by walking the project's own src dir, not attacker input
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0600) // #nosec G703 -- target is derived from walking the project's own src dir, not attacker input
	})
}

// copyCSSFiles copies all .css files found under projectDir (skipping dist/ and public/)
// into distDir, flattening them to a single level.
// This mirrors React's co-located CSS pattern: put Component.css next to Component.vane
// and vane build will include it in the output.
func copyCSSFiles(projectDir, distDir string) error {
	skipDirs := map[string]bool{"dist": true, "public": true}
	return filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".css") {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 G122 -- path is discovered by walking the developer's own project dir, not attacker input
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(distDir, filepath.Base(path)), data, 0600) // #nosec G703 -- distDir is the project's own dist/ directory
	})
}

// cssImportRe matches a standalone `@import 'File.css';` line, the format
// styleCSSTemplate/vane-page's public/style.css uses to pull in each
// co-located Component.css by name.
var cssImportRe = regexp.MustCompile(`(?m)^@import\s+['"]([^'"]+)['"]\s*;[ \t]*\r?\n?`)

// bundleCSS collapses dist/style.css's @import chain into a single file and
// deletes the now-redundant per-file copies, release builds only.
// copyCSSFiles above already flattened every co-located .css into distDir,
// so each @import target sits right next to style.css. Without this, a
// real vane app ships one render-blocking request per component (measured
// 590-1930ms wasted on vane-page's own Lighthouse run, ~20 requests
// before first paint), and even after inlining, the loose files would
// still sit in dist/ unused, so they're removed once their contents are
// folded into style.css. Dev (vane run) never calls this: hot-reload's
// OnCSSChange watches the individual co-located files, and bundling here
// would break the "edit Button.css, see it instantly" loop.
func bundleCSS(distDir string) error {
	stylePath := filepath.Join(distDir, "style.css")
	data, err := os.ReadFile(stylePath) // #nosec G304 -- stylePath is the project's own dist/style.css, not attacker input
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	matches := cssImportRe.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		return nil
	}

	var bundled strings.Builder
	imported := make([]string, 0, len(matches))
	for _, m := range matches {
		name := filepath.Base(m[1])
		content, err := os.ReadFile(filepath.Join(distDir, name)) // #nosec G304 -- import target resolved against the project's own dist/, base-name only
		if err != nil {
			return fmt.Errorf("import %q: %w", m[1], err)
		}
		bundled.Write(content)
		bundled.WriteString("\n")
		imported = append(imported, name)
	}
	bundled.WriteString(cssImportRe.ReplaceAllString(string(data), ""))

	if err := os.WriteFile(stylePath, []byte(bundled.String()), 0600); err != nil { // #nosec G703 -- stylePath is the project's own dist/style.css
		return err
	}

	// The individual files are now dead weight: nothing in dist/ links to
	// them once their contents live in style.css, and shipping them anyway
	// would defeat the point of bundling (still N requests reachable, just
	// unused ones).
	for _, name := range imported {
		if err := os.Remove(filepath.Join(distDir, name)); err != nil { // #nosec G703 -- name is one of the files this function just read from distDir above
			return err
		}
	}

	return nil
}

// goModTemplate returns the go.mod for a scaffolded project. It has no
// require for github.com/filipejohansson/vane yet; the `go mod tidy` call
// at the end of cmdInit adds it, resolved to the latest published version.
func goModTemplate(module string) string {
	return fmt.Sprintf(`module %s

go 1.24
`, module)
}

func gitignoreTemplate() string {
	return `dist/
*_vane.go
`
}

func indexHTMLTemplate(name string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>
  <link rel="icon" type="image/svg+xml" href="favicon.svg">
  <link rel="stylesheet" href="style.css">
  <script src="wasm_exec.js"></script>
  <script src="boot.js"></script>
</head>
<body>
  <div id="root"></div>
</body>
</html>
`, name)
}

// bootJSTemplate is the hash-redirect + WASM-boot code indexHTMLTemplate
// points at via a same-origin <script src>. Kept out of index.html so an
// app's CSP can use script-src 'self' (plus 'wasm-unsafe-eval' for the
// instantiate call) instead of being forced into 'unsafe-inline', which
// would defeat most of what a CSP buys against XSS.
func bootJSTemplate() string {
	return `const base = new URL(".", document.currentScript.src).pathname
if (!location.hash && location.pathname !== base) {
  history.replaceState(null, "", base + "#" + location.pathname.slice(base.length) + location.search)
}

const go = new Go()
WebAssembly.instantiateStreaming(fetch("app.wasm"), go.importObject)
  .then(r => go.run(r.instance))
`
}

func mainGoTemplate() string {
	return `//go:build js && wasm

package main

import (
	"github.com/filipejohansson/vane/core"
)

func main() {
	core.Mount("root", App)
	select {}
}
`
}

func appVaneTemplate(mod string) string {
	return fmt.Sprintf(`package main

import (
	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/core/router"
	"%s/src/components"
	"%s/src/pages"
)

func App() core.Node {
	return (
		<div className="app">
			<nav className="nav">
				<a href="#/" className="nav-brand">Vane</a>
				<div className="nav-links">
					{components.NavLink("/", "Home")}
					{components.NavLink("/about", "About")}
				</div>
			</nav>
			<main className="main">
				{router.Router(
					router.Route("/", pages.Home, "Home - Vane starter"),
					router.Route("/about", pages.About, "About - Vane starter"),
				)}
			</main>
		</div>
	)
}
`, mod, mod)
}

func homeVaneTemplate(mod string) string {
	return fmt.Sprintf(`package pages

import (
	"github.com/filipejohansson/vane/core"
	"%s/src/store"
)

func Home() core.Node {
	return (
		<div className="page">
			<h1 className="page-title">Build web apps in Go.</h1>
			<p className="page-desc">
				Vane compiles your Go + JSX to WebAssembly.
			</p>
			<div className="demo">
				<label className="demo-label" htmlFor="name-input">Try it, type your name:</label>
				<input
					id="name-input"
					className="input"
					placeholder="Your name"
					value={store.Name.Get()}
					onInput={func(e core.InputEvent) { store.Name.Set(e.Value) }}
				/>
				{func() core.Node {
					if n := store.Name.Get(); n != "" {
						return (<p className="greeting">Hello, {n}!</p>)
					}
					return nil
				}()}
			</div>
		</div>
	)
}
`, mod)
}

func aboutVaneTemplate(mod string) string {
	return fmt.Sprintf(`package pages

import (
	"github.com/filipejohansson/vane/core"
	"%s/src/store"
)

type feature struct {
	name string
	desc string
}

func About() core.Node {
	features := []feature{
		{"Signals", "Fine-grained reactivity. Only what changed re-renders."},
		{"JSX in Go", "Familiar component syntax, full Go type safety."},
		{"Router", "Hash-based SPA routing with layouts and nested routes."},
		{"Store", "Global state management with signals and computed values."},
	}

	return (
		<div className="page">
			<h1 className="page-title">About Vane</h1>
			{func() core.Node {
				if n := store.Name.Get(); n != "" {
					return (<p className="page-desc">Nice to meet you, {n}.</p>)
				}
				return nil
			}()}
			<p className="page-desc">
				Vane is a reactive frontend framework for Go. Write components in Go + JSX syntax,
				compile to WebAssembly, ship to any browser.
			</p>
			<ul className="feature-list">
				{for _, f := range features {
					<li><strong>{f.name}</strong>: {f.desc}</li>
				}}
			</ul>
			<p className="page-desc">
				Run <code>vane build .</code> to compile, <code>vane run .</code> to preview.
			</p>
		</div>
	)
}
`, mod)
}

func navLinkVaneTemplate() string {
	return `package components

import (
	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/core/router"
)

func NavLink(to, label string) core.Node {
	return router.ActiveLink(
		router.ActiveLinkProps{
			LinkProps: router.LinkProps{
				To: to,
				HTMLProps: core.HTMLProps{
					Class: "nav-link",
				},
			},
			ActiveClass: "nav-link--active",
		},
		label,
	)
}
`
}

func storeUserGoTemplate() string {
	return `//go:build js && wasm

package store

import "github.com/filipejohansson/vane/core"

var Name = core.NewSignal("")
`
}

// faviconSVGTemplate is the Vane logo mark, scaffolded
// into every new project so it doesn't ship with a blank browser tab icon
func faviconSVGTemplate() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 500 500" width="500" height="500">
<path d="M300.1 323.5C266.07 323.43 232.03 323.36 197.99 323.29C162.16 218.94 126.33 114.6 90.5 10.25C116.18 10.25 141.86 10.25 167.54 10.25C194.78 97.65 222.01 185.05 249.25 272.44C276.42 185.05 303.58 97.65 330.75 10.25C356.41 10.22 382.08 10.2 407.75 10.17C371.87 114.62 335.98 219.06 300.1 323.5ZM244 340.49C247.67 340.49 251.33 340.49 255 340.49C267.68 340.87 280.36 350.34 286.75 360.97C288.1 363.22 292.16 374.56 292.78 375.01C296.27 376.72 307.26 375.5 311.75 375.5C327.25 375.5 342.75 375.5 358.25 375.5C362.92 375.5 367.58 375.5 372.25 375.5C374.02 375.5 376.72 375.97 377.85 374.85C379.19 373.49 378.5 369.36 378.5 367.25C378.5 359.96 378.62 352.65 378.5 345.36C386.58 347.23 394.75 351.63 402.46 354.75C417.56 360.87 432.87 366.51 447.91 372.8C453.82 375.27 459.83 377.63 465.83 379.88C468.32 380.81 472.36 381.64 473.91 383.75C442.1 396.38 410.3 409.01 378.5 421.64C378.62 414.35 378.5 407.04 378.5 399.75C378.5 397.64 379.19 393.51 377.85 392.15C376.72 391.03 374.02 391.5 372.25 391.5C367.58 391.5 362.92 391.5 358.25 391.5C342.75 391.5 327.25 391.5 311.75 391.5C307.26 391.5 296.27 390.28 292.78 391.99C291.41 392.98 291.32 395.98 290.77 397.57C289.57 401.04 287.84 404.5 285.79 407.56C279.32 417.21 269.95 422.68 259.15 426.13C257.56 427.76 258.5 433.71 258.5 436.25C258.5 445.75 258.5 455.25 258.5 464.75C258.5 467.43 257.45 474.12 259.15 475.85C260.4 477.08 263.81 476.5 265.75 476.5C271.11 476.5 277.28 475.69 282.5 476.75C282.42 480.67 282.33 484.59 282.25 488.5C260.67 488.42 239.08 488.33 217.5 488.25C217.58 484.33 217.67 480.41 217.75 476.5C223.42 476.5 229.08 476.5 234.75 476.5C236.61 476.5 239.66 477.02 240.85 475.85C242.6 474.07 241.5 467.01 241.5 464.25C241.5 455.42 241.5 446.58 241.5 437.75C241.49 434.55 242.33 429.49 241.01 426.78C239.76 425.06 234.97 424.63 232.9 423.79C227.81 421.72 223.08 418.44 219.11 414.64C214.91 410.62 211.38 405.47 209.21 400.11C208.37 398.04 207.94 393.24 206.22 391.99C174.48 391.98 142.74 391.98 111 391.98C108.01 392.91 105.68 398.14 103.86 400.63C98.88 407.45 94.42 415.5 88.65 421.62C70.21 421.62 51.77 421.63 33.33 421.63C32.93 419.74 38.34 413.4 39.72 411.48C44.53 404.77 49.26 398.02 54.11 391.35C55.52 389.41 59.03 386.04 59.24 383.63C59.44 381.41 55.55 377.69 54.31 375.95C49.18 368.74 44.08 361.51 38.88 354.36C37.42 352.34 32.83 347.76 33.33 345.37C51.77 345.37 70.21 345.38 88.65 345.38C94.42 351.5 98.88 359.55 103.86 366.37C105.68 368.86 108.01 374.09 111 375.02C142.74 375.02 174.48 375.02 206.22 375.01C207.55 374.05 210.01 364.94 211.32 362.56C217.59 351.17 230.66 340.89 244 340.49ZM245.89 358.23C212.85 363.43 220.57 413.97 253.11 408.77C286.15 403.5 278.46 353.1 245.89 358.23Z" fill="#00add8" fill-rule="evenodd" stroke="#00add8" stroke-width="0.25" stroke-linejoin="round"/>
<path d="M197.99 323.29C232.03 323.36 266.07 323.43 300.1 323.5C297.51 325.38 282.85 324.17 278.75 324.17C259.42 324.17 240.08 324.17 220.75 324.17C215.25 324.17 209.75 324.17 204.25 324.17C202.18 324.17 199.36 324.72 197.99 323.29ZM255 340.49C251.33 340.49 247.67 340.49 244 340.49C245.99 339.2 253.01 339.19 255 340.49ZM88.65 345.38C70.21 345.38 51.77 345.37 33.33 345.37C51.77 345.37 70.21 345.38 88.65 345.38ZM378.5 345.36C378.62 352.65 378.5 359.96 378.5 367.25C378.5 369.36 379.19 373.49 377.85 374.85C377.48 367.95 377.82 360.74 377.82 353.75C377.82 351.25 377.13 347.33 378.5 345.36ZM111 375.02C142.74 375.02 174.48 375.02 206.22 375.01C174.48 375.02 142.74 375.02 111 375.02ZM292.78 375.01C321.14 374.96 349.5 374.91 377.85 374.85C376.72 375.97 374.02 375.5 372.25 375.5C367.58 375.5 362.92 375.5 358.25 375.5C342.75 375.5 327.25 375.5 311.75 375.5C307.26 375.5 296.27 376.72 292.78 375.01ZM206.22 391.99C174.48 391.98 142.74 391.98 111 391.98C142.74 391.98 174.48 391.98 206.22 391.99ZM377.85 392.15C349.5 392.09 321.14 392.04 292.78 391.99C296.27 390.28 307.26 391.5 311.75 391.5C327.25 391.5 342.75 391.5 358.25 391.5C362.92 391.5 367.58 391.5 372.25 391.5C374.02 391.5 376.72 391.03 377.85 392.15ZM377.85 392.15C379.19 393.51 378.5 397.64 378.5 399.75C378.5 407.04 378.62 414.35 378.5 421.64C377.13 419.67 377.82 415.75 377.82 413.25C377.82 406.26 377.48 399.05 377.85 392.15ZM33.33 421.63C51.77 421.63 70.21 421.62 88.65 421.62C70.21 421.62 51.77 421.63 33.33 421.63ZM259.15 426.13C259.15 442.7 259.15 459.28 259.15 475.85C257.45 474.12 258.5 467.43 258.5 464.75C258.5 455.25 258.5 445.75 258.5 436.25C258.5 433.71 257.56 427.76 259.15 426.13ZM241.01 426.78C242.33 429.49 241.49 434.55 241.5 437.75C241.5 446.58 241.5 455.42 241.5 464.25C241.5 467.01 242.6 474.07 240.85 475.85C240.91 459.5 240.96 443.14 241.01 426.78ZM240.85 475.85C239.66 477.02 236.61 476.5 234.75 476.5C229.08 476.5 223.42 476.5 217.75 476.5C217.67 480.41 217.58 484.33 217.5 488.25C239.08 488.33 260.67 488.42 282.25 488.5C282.33 484.59 282.42 480.67 282.5 476.75C277.28 475.69 271.11 476.5 265.75 476.5C263.81 476.5 260.4 477.08 259.15 475.85C267.12 475.99 275.1 476.12 283.07 476.25C282.96 480.52 282.86 484.8 282.75 489.07C260.81 488.96 238.87 488.86 216.93 488.75C217.04 484.48 217.14 480.2 217.25 475.93C225.12 475.9 232.99 475.88 240.85 475.85Z" fill="#42a2ba" fill-rule="evenodd" stroke="#42a2ba" stroke-width="0.25" stroke-linejoin="round"/>
</svg>
`
}

func styleCSSTemplate() string {
	return `*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

:root {
  --bg: #0f1117;
  --surface: #1e2130;
  --surface-2: #2d3348;
  --surface-3: #363d56;
  --border: #3d4566;
  --border-2: #4a5275;

  --text: #e2e8f0;
  --muted: #94a3b8;
  --hint: #64748b;

  --primary: #00ADD8;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  background: var(--bg);
  color: var(--text);
  min-height: 100vh;
}

/** layout */

.app {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.nav {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 1.5rem;
  height: 3.25rem;
  border-bottom: 1px solid var(--border);
  position: sticky;
  top: 0;
  background: var(--bg);
  z-index: 10;
}

.nav-brand {
  font-weight: 700;
  font-size: 1.1rem;
  color: var(--primary);
  text-decoration: none;
  letter-spacing: -0.01em;
}

.nav-links {
  display: flex;
  gap: 0.25rem;
}

.nav-link {
  color: var(--muted);
  text-decoration: none;
  font-size: 0.9rem;
  padding: 0.35rem 0.75rem;
  border-radius: 6px;
  transition: color 0.15s, background 0.15s;
}

.nav-link:hover { color: var(--text); background: var(--surface); }
.nav-link--active { color: var(--primary); }

.main {
  flex: 1;
  display: flex;
  justify-content: center;
  padding: 3rem 1.5rem 4rem;
}

/** page */

.page {
  width: 100%;
  max-width: 640px;
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.page-title {
  font-size: 2rem;
  font-weight: 700;
  letter-spacing: -0.03em;
  line-height: 1.2;
}

.page-desc {
  color: var(--muted);
  line-height: 1.7;
}

/** demo block */

.demo {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin-top: 0.5rem;
}

.demo-label {
  font-size: 0.875rem;
  color: var(--hint);
}

.greeting {
  font-size: 1.25rem;
  font-weight: 600;
  color: var(--primary);
}

/** feature list */

.feature-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding-left: 1.25rem;
  color: var(--muted);
  line-height: 1.6;
}

.feature-list strong { color: var(--text); }

/** form controls */

.input {
  width: 100%;
  max-width: 320px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  color: var(--text);
  padding: 0.5rem 0.75rem;
  font-size: 0.9rem;
  outline: none;
  transition: border-color 0.15s;
}

.input:focus { border-color: var(--primary); }
.input::placeholder { color: var(--hint); }

code {
  background: var(--surface);
  color: var(--primary);
  padding: 0.15rem 0.4rem;
  border-radius: 4px;
  font-size: 0.875rem;
}

/** responsive */

@media (max-width: 480px) {
  .nav { padding: 0 1rem; }
  .main { padding: 2rem 1rem 3rem; }
  .page-title { font-size: 1.5rem; }
  .input { max-width: 100%; }
}
`
}

// printBuildDiagnostics parses go build stderr and renders each .vane error with
// a source snippet. Unrecognized lines are printed indented in red.
func printBuildDiagnostics(stderr string) {
	re := regexp.MustCompile(`^(.*\.vane):(\d+)(?::(\d+))?: (.+)$`)
	for _, raw := range strings.Split(strings.TrimRight(stderr, "\n"), "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		m := re.FindStringSubmatch(line)
		if m == nil {
			fmt.Fprintf(os.Stderr, "  %s%s%s\n", clRed, line, clReset)
			continue
		}
		if m[4] == "too many errors" {
			continue
		}
		lineNum, _ := strconv.Atoi(m[2])
		colNum, _ := strconv.Atoi(m[3])
		printBuildSnippet(m[1], lineNum, colNum, m[4], buildErrorHint(m[4]))
	}
	fmt.Fprintln(os.Stderr)
}

// buildErrorHint recognizes specific go build error messages that stem from
// a vane-syntax mistake one level removed from the vane compiler itself
// (the generated code compiles, but with a type the caller didn't mean to
// write), and points at the vane-level fix instead of leaving a bare Go
// type-mismatch as the only signal.
func buildErrorHint(msg string) string {
	if strings.Contains(msg, "core.Style value in return statement") {
		return "style={expr} compiles to core.DynStyle, which needs a core.Style struct (e.g. style={core.Style{Color: \"red\"}}), not a string. For a literal CSS string, drop the braces: style=\"color:red\" instead of style={someStringVar}."
	}
	return ""
}

func printBuildSnippet(file string, lineNum, colNum int, msg, hint string) {
	loc := strconv.Itoa(lineNum)
	if colNum > 0 {
		loc = fmt.Sprintf("%d:%d", lineNum, colNum)
	}
	fmt.Fprintf(os.Stderr, "  %s%s:%s%s %s·%s %s%s%s\n",
		clBold, file, loc, clReset,
		clDim, clReset,
		clRed+clBold, msg, clReset)

	src, err := os.ReadFile(file) // #nosec G304 -- file is parsed from go build's own stderr output, referencing the project's own source files
	if err != nil {
		fmt.Fprintln(os.Stderr)
		return
	}
	srcLines := strings.Split(string(src), "\n")
	first := max(0, lineNum-3)
	last := min(len(srcLines), lineNum+2)
	numWidth := len(strconv.Itoa(last))

	for i := first; i < last; i++ {
		ln := i + 1
		isError := ln == lineNum

		rawContent := ""
		if i < len(srcLines) {
			rawContent = srcLines[i]
		}
		content := strings.ReplaceAll(rawContent, "\t", "  ")

		var row strings.Builder
		row.WriteString("  ")
		if isError {
			row.WriteString(clBold)
		} else {
			row.WriteString(clDim)
		}
		fmt.Fprintf(&row, "%*d", numWidth, ln)
		fmt.Fprintf(&row, "%s ", clReset)
		if isError {
			fmt.Fprintf(&row, "%s→%s", clRed, clReset)
		} else {
			fmt.Fprintf(&row, "%s %s", clDim, clReset)
		}
		fmt.Fprintf(&row, "%s│%s ", clDim, clReset)
		row.WriteString(content)
		row.WriteString("\n")
		fmt.Fprint(os.Stderr, row.String())

		if isError && colNum > 0 {
			visualPad := tabExpandedWidth(rawContent, colNum-1)
			fmt.Fprintf(os.Stderr, "  %s  %s│%s %s%s^%s\n",
				strings.Repeat(" ", numWidth),
				clDim, clReset,
				clRed, strings.Repeat(" ", visualPad), clReset)
		}
	}
	if hint != "" {
		fmt.Fprintf(os.Stderr, "  %s%sHint:%s %s\n", clYellow, clBold, clReset, hint)
	}
	fmt.Fprintln(os.Stderr)
}

func tabExpandedWidth(line string, bytePos int) int {
	visual := 0
	for i := 0; i < bytePos && i < len(line); i++ {
		if line[i] == '\t' {
			visual += 2
		} else {
			visual++
		}
	}
	return visual
}

func findModuleRoot(dir string) (string, error) {
	abs := dir
	for {
		if _, err := os.Stat(filepath.Join(abs, "go.mod")); err == nil {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("no go.mod found above %s", dir)
		}
		abs = parent
	}
}
