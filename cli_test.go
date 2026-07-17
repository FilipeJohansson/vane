package main_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// vaneBin is the path to the compiled vane binary used by all CLI tests.
var vaneBin string

func TestMain(m *testing.M) {
	bin, cleanup, err := buildVane()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build vane: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()
	vaneBin = bin
	os.Exit(m.Run())
}

func buildVane() (bin string, cleanup func(), err error) {
	tmp, err := os.MkdirTemp("", "vane-test-bin-*")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.RemoveAll(tmp) }

	bin = filepath.Join(tmp, "vane")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("%w\n%s", err, out)
	}
	return bin, cleanup, nil
}

func runVane(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(vaneBin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func vaneRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

// newProject creates an empty dir under the vane root, runs "vane init
// <module>" inside it, and registers cleanup of the created directory.
// Returns the absolute path of the created project directory.
func newProject(t *testing.T, name string) string {
	t.Helper()
	root := vaneRoot(t)
	appDir := filepath.Join(root, name)
	t.Cleanup(func() { os.RemoveAll(appDir) })

	if err := os.Mkdir(appDir, 0750); err != nil {
		t.Fatalf("mkdir %s: %v", appDir, err)
	}
	module := "example.com/" + name
	out, err := runVane(t, appDir, "init", module)
	if err != nil {
		t.Fatalf("vane init %s: %v\n%s", module, err, out)
	}
	return appDir
}

// uniqueName returns a short unique project name safe for use as a directory.
func uniqueName(prefix string) string {
	return prefix + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// pinToLocalVane points a scaffolded project's go.mod at this checkout of
// github.com/filipejohansson/vane instead of the published module, so tests
// that build the project exercise the code under test (not whatever's on
// the network) and don't depend on the vane repo being publicly fetchable.
func pinToLocalVane(t *testing.T, appDir string) {
	t.Helper()
	root := vaneRoot(t)

	editArgs := []string{"mod", "edit",
		"-require=github.com/filipejohansson/vane@v0.0.0",
		"-replace=github.com/filipejohansson/vane=" + root,
	}
	editCmd := exec.Command("go", editArgs...)
	editCmd.Dir = appDir
	if out, err := editCmd.CombinedOutput(); err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(editArgs, " "), err, out)
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = appDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}
}

// TestVaneInit verifies that "vane init <module>" creates the expected project scaffold.
func TestVaneInit(t *testing.T) {
	name := uniqueName("tst")
	appDir := newProject(t, name)

	required := []string{
		"go.mod",
		".gitignore",
		"main.go",
		"App.vane",
		filepath.Join("public", "index.html"),
		filepath.Join("public", "boot.js"),
		filepath.Join("public", "style.css"),
		filepath.Join("public", "favicon.svg"),
		filepath.Join("public", "wasm_exec.js"),
		filepath.Join("src", "pages", "Home.vane"),
		filepath.Join("src", "pages", "About.vane"),
		filepath.Join("src", "components", "NavLink.vane"),
		filepath.Join("src", "store", "user.go"),
	}
	for _, rel := range required {
		if _, statErr := os.Stat(filepath.Join(appDir, rel)); os.IsNotExist(statErr) {
			t.Errorf("expected file missing: %s", rel)
		}
	}

	gomod, _ := os.ReadFile(filepath.Join(appDir, "go.mod"))
	if !strings.Contains(string(gomod), "module example.com/"+name) {
		t.Errorf("go.mod has wrong module name:\n%s", gomod)
	}
	if strings.Contains(string(gomod), "replace github.com/filipejohansson/vane =>") {
		t.Errorf("go.mod should not contain a local replace directive:\n%s", gomod)
	}

	gitignore, _ := os.ReadFile(filepath.Join(appDir, ".gitignore"))
	if !strings.Contains(string(gitignore), "dist/") {
		t.Errorf(".gitignore should ignore dist/, got:\n%s", gitignore)
	}
}

// TestVaneInitNonEmptyDirFails verifies that "vane init" refuses to run in a
// directory that already has content, mirroring `go mod init`'s expectation
// that mkdir/cd happens before init, not as part of it.
func TestVaneInitNonEmptyDirFails(t *testing.T) {
	root := vaneRoot(t)
	appDir := filepath.Join(root, uniqueName("tst"))
	if err := os.Mkdir(appDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(appDir) })
	if err := os.WriteFile(filepath.Join(appDir, "existing.txt"), []byte("hi"), 0600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	out, err := runVane(t, appDir, "init", "example.com/whatever")
	if err == nil {
		t.Fatalf("expected error on non-empty dir, got none\noutput: %s", out)
	}
}

// TestVaneCompile verifies that "vane compile <file.vane>" produces valid Go source.
func TestVaneCompile(t *testing.T) {
	root := vaneRoot(t)
	src := filepath.Join(root, "examples", "reactive-showcase", "App.vane")

	out, err := runVane(t, root, "compile", src)
	if err != nil {
		t.Fatalf("vane compile failed: %v\n%s", err, out)
	}

	fset := token.NewFileSet()
	if _, parseErr := parser.ParseFile(fset, "", out, parser.AllErrors); parseErr != nil {
		t.Fatalf("output is not valid Go: %v\noutput:\n%s", parseErr, out)
	}

	for _, want := range []string{
		"//go:build js && wasm",
		"package main",
		"func App()",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

// TestVaneCompileNonVaneFile verifies that "vane compile" rejects non-.vane files.
func TestVaneCompileNonVaneFile(t *testing.T) {
	root := vaneRoot(t)
	out, err := runVane(t, root, "compile", "main.go")
	if err == nil {
		t.Fatalf("expected error for non-.vane file, got none\noutput: %s", out)
	}
}

// TestVaneCompileSyntaxError verifies that compile errors reference the source file and line.
func TestVaneCompileSyntaxError(t *testing.T) {
	tmp := t.TempDir()

	bad := filepath.Join(tmp, "Bad.vane")
	os.WriteFile(bad, []byte(`package main

import "syscall/js"

func Bad() js.Value {
	return (<div><span></div>)
}
`), 0644)

	out, err := runVane(t, tmp, "compile", bad)
	if err == nil {
		t.Fatalf("expected compile error, got none\noutput: %s", out)
	}
	if !strings.Contains(out, "Bad.vane") {
		t.Errorf("error should reference source file name, got:\n%s", out)
	}
}

// TestVaneCompileAllExamples verifies each example .vane file compiles without error.
func TestVaneCompileAllExamples(t *testing.T) {
	root := vaneRoot(t)

	vaneFiles := []string{
		filepath.Join("examples", "reactive-showcase", "App.vane"),
		filepath.Join("examples", "shared-store", "App.vane"),
		filepath.Join("examples", "shared-store", "src", "components", "Login.vane"),
		filepath.Join("examples", "shared-store", "src", "components", "Navbar.vane"),
		filepath.Join("examples", "component-api", "App.vane"),
		filepath.Join("examples", "component-api", "src", "components", "Modal.vane"),
		filepath.Join("examples", "component-api", "src", "components", "Toaster.vane"),
		filepath.Join("examples", "forms-and-lists", "App.vane"),
		filepath.Join("examples", "forms-and-lists", "src", "pages", "Home.vane"),
		filepath.Join("examples", "forms-and-lists", "src", "pages", "About.vane"),
		filepath.Join("examples", "forms-and-lists", "src", "pages", "Contact.vane"),
		filepath.Join("examples", "forms-and-lists", "src", "pages", "NotFound.vane"),
	}

	for _, rel := range vaneFiles {
		rel := rel
		t.Run(filepath.Base(rel), func(t *testing.T) {
			t.Parallel()
			out, err := runVane(t, root, "compile", filepath.Join(root, rel))
			if err != nil {
				t.Fatalf("vane compile %s: %v\n%s", rel, err, out)
			}
			fset := token.NewFileSet()
			if _, parseErr := parser.ParseFile(fset, "", out, parser.AllErrors); parseErr != nil {
				t.Fatalf("%s: output is not valid Go: %v", rel, parseErr)
			}
		})
	}
}

// TestVaneInitThenBuild is a full end-to-end test: scaffold a new project and build it to WASM.
func TestVaneInitThenBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping WASM build in short mode")
	}

	appDir := newProject(t, uniqueName("tst"))
	pinToLocalVane(t, appDir)

	out, err := runVane(t, appDir, "build", ".")
	if err != nil {
		t.Fatalf("vane build: %v\n%s", err, out)
	}

	wasmPath := filepath.Join(appDir, "dist", "app.wasm")
	info, statErr := os.Stat(wasmPath)
	if statErr != nil {
		t.Fatalf("dist/app.wasm not created: %v", statErr)
	}
	const minWasmSize = 1 * 1024 * 1024 // release Go WASM is never <1 MB
	if info.Size() < minWasmSize {
		t.Errorf("dist/app.wasm suspiciously small: %d bytes (min expected %d)", info.Size(), minWasmSize)
	}

	for _, rel := range []string{
		filepath.Join("dist", "index.html"),
		filepath.Join("dist", "wasm_exec.js"),
		filepath.Join("dist", "boot.js"),
		filepath.Join("dist", "style.css"),
		filepath.Join("dist", "favicon.svg"),
	} {
		if _, err := os.Stat(filepath.Join(appDir, rel)); err != nil {
			t.Errorf("expected dist file missing: %s", rel)
		}
	}
}

// TestVaneBuildExamples verifies every example builds to a valid WASM binary.
func TestVaneBuildExamples(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping WASM build in short mode")
	}

	root := vaneRoot(t)
	examples := []string{
		"reactive-showcase",
		"shared-store",
		"component-api",
		"forms-and-lists",
	}

	for _, ex := range examples {
		ex := ex
		t.Run(ex, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(root, "examples", ex)

			out, err := runVane(t, dir, "build", ".")
			if err != nil {
				t.Fatalf("vane build %s: %v\n%s", ex, err, out)
			}

			wasmPath := filepath.Join(dir, "dist", "app.wasm")
			info, statErr := os.Stat(wasmPath)
			if statErr != nil {
				t.Fatalf("%s: dist/app.wasm not found: %v", ex, statErr)
			}
			const minWasmSize = 1 * 1024 * 1024
			if info.Size() < minWasmSize {
				t.Errorf("%s: dist/app.wasm too small: %d bytes", ex, info.Size())
			}
		})
	}
}

// TestVaneBuildRelease verifies "vane build --release" succeeds and actually
// changes the build (via -ldflags -X core.releaseFlag=1) rather than silently
// behaving like a normal build. Regression guard for the --release flag
// threaded through cmdBuild → buildWasm → go build args.
func TestVaneBuildRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping WASM build in short mode")
	}

	dir := filepath.Join(vaneRoot(t), "examples", "routing")
	wasmPath := filepath.Join(dir, "dist", "app.wasm")

	out, err := runVane(t, dir, "build", ".")
	if err != nil {
		t.Fatalf("vane build: %v\n%s", err, out)
	}
	normalWasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Fatalf("reading normal build output: %v", err)
	}

	out, err = runVane(t, dir, "build", ".", "--release")
	if err != nil {
		t.Fatalf("vane build --release: %v\n%s", err, out)
	}
	releaseWasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Fatalf("reading release build output: %v", err)
	}

	if len(releaseWasm) == 0 {
		t.Fatal("release build produced an empty app.wasm")
	}
	// -ldflags -X sets a string var, which changes the compiled binary's bytes.
	// If --release silently did nothing, the two builds would be byte-identical.
	if string(normalWasm) == string(releaseWasm) {
		t.Error("--release build is byte-identical to a normal build: releaseFlag likely not wired into go build args")
	}
}

// TestVaneBuildTinygoRelease is the --tinygo counterpart to
// TestVaneBuildRelease: "vane build --tinygo --release" used to silently
// ignore --release entirely (cmdBuildTinyGo never received the flag), so a
// "release" TinyGo binary was byte-identical to a dev one, core.Warn calls
// included. Skipped when TinyGo isn't installed, same optional-dependency
// pattern as the rest of the tinygo.go build path.
func TestVaneBuildTinygoRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping WASM build in short mode")
	}
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("tinygo not installed")
	}

	dir := filepath.Join(vaneRoot(t), "examples", "routing")
	wasmPath := filepath.Join(dir, "dist", "app.wasm")

	out, err := runVane(t, dir, "build", ".", "--tinygo")
	if err != nil {
		t.Fatalf("vane build --tinygo: %v\n%s", err, out)
	}
	normalWasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Fatalf("reading normal tinygo build output: %v", err)
	}

	out, err = runVane(t, dir, "build", ".", "--tinygo", "--release")
	if err != nil {
		t.Fatalf("vane build --tinygo --release: %v\n%s", err, out)
	}
	releaseWasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Fatalf("reading tinygo release build output: %v", err)
	}

	if len(releaseWasm) == 0 {
		t.Fatal("tinygo release build produced an empty app.wasm")
	}
	if string(normalWasm) == string(releaseWasm) {
		t.Error("--tinygo --release build is byte-identical to a normal --tinygo build: releaseFlag likely not wired into tinygo build args")
	}
}
