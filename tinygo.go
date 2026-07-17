package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/filipejohansson/vane/internal/compiler"
)

// reorderValueFlags lists flags that take their value as a separate
// following token (e.g. "--port 8080"), as opposed to a bare boolean flag
// like "--tinygo". reorderArgs needs this list to keep such a pair
// together when it moves flags ahead of positional args.
var reorderValueFlags = map[string]bool{"--port": true}

// reorderArgs moves flag arguments (starting with '-') before positional
// arguments so that flag.Parse works regardless of the order the user typed
// them. Example: ["./myapp", "--tinygo"] → ["--tinygo", "./myapp"]. A flag
// listed in reorderValueFlags keeps its following value token attached, so
// ["./myapp", "--port", "8080"] → ["--port", "8080", "./myapp"] rather than
// separating the flag from its value.
func reorderArgs(args []string) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
			continue
		}
		flags = append(flags, a)
		if reorderValueFlags[a] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, pos...)
}

// cmdBuildTinyGo compiles .vane files and builds WASM with TinyGo.
func cmdBuildTinyGo(dir string, release, skipOptimize bool) error {
	return buildWasmTinyGo(dir, false, "", release, skipOptimize)
}

// tinygoBuildArgs builds the "tinygo build" argument list for compiling
// app.wasm. release adds the same core.releaseFlag ldflag wasmBuildArgs sets
// for the standard Go build path, so core.Warn calls are stripped
// consistently regardless of which compiler produced the binary. Unlike
// wasmBuildArgs there is no "-s -w" to add: TinyGo already strips debug info
// by default outside of debug builds (see the -no-debug=false override
// below), so release has nothing extra to do there.
func tinygoBuildArgs(wasmOut string, debug, release bool) []string {
	args := []string{"build", "-target", "wasm"}
	if debug {
		args = append(args, "-no-debug=false", "-opt=1")
	}
	if release {
		args = append(args, "-ldflags=-X github.com/filipejohansson/vane/core.releaseFlag=1")
	}
	args = append(args, "-o", wasmOut, ".")
	return args
}

// buildWasmTinyGo compiles .vane → _vane.go files alongside sources (TinyGo has
// no -overlay support), runs tinygo build, cleans up the generated files, then
// copies TinyGo's wasm_exec.js over the standard Go version in dist/.
// //line directives always use the absolute path of the original .vane file so
// TinyGo's DWARF output embeds a path Chrome can open as file:///... directly.
// HTTP URLs do not work: LLVM treats //line values as file paths, not URLs.
// release/skipOptimize mirror buildWasm's flags of the same name: release
// sets core.releaseFlag via -ldflags (see tinygoBuildArgs) and, unless
// skipOptimize is set, runs the same wasm-opt -Oz pass buildWasm applies to
// standard Go builds. Previously these flags were silently ignored for
// --tinygo builds, `vane build --tinygo --release` produced a binary
// indistinguishable from a dev build.
func buildWasmTinyGo(dir string, debug bool, sourceURLBase string, release, skipOptimize bool) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	skipDirs := map[string]bool{"dist": true, "public": true}
	var written []string

	// Always clean up generated _vane.go files, even on build failure.
	defer func() {
		for _, p := range written {
			_ = os.Remove(p)
		}
	}()

	n := 0
	walkErr := filepath.WalkDir(abs, func(path string, d fs.DirEntry, e error) error {
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

		// Always use the absolute path of the original .vane file.
		// TinyGo's LLVM backend embeds //line values verbatim in DWARF, it
		// does not resolve HTTP URLs. Absolute paths let Chrome open the file
		// directly as file:///..., which works because .vane files are on disk.
		lineFilename := filepath.ToSlash(path)

		goSrc, err := compiler.Compile(string(src), lineFilename)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}

		goPath := strings.TrimSuffix(path, ".vane") + "_vane.go"
		if err := os.WriteFile(goPath, []byte(goSrc), 0o600); err != nil { // #nosec G703 G122 -- goPath is derived from a path found while walking the developer's own project dir, not attacker input
			return err
		}
		written = append(written, goPath)
		fmt.Printf("compiled: %s\n", filepath.Base(path))
		n++
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	if n > 0 {
		fmt.Printf("compiled %d .vane file(s)\n", n)
	}

	// Locate main package (mirrors buildWasm logic).
	mainPkgDir := abs
	for _, candidate := range []string{"src", "."} {
		p := filepath.Join(abs, candidate, "main.go")
		if candidate == "." {
			p = filepath.Join(abs, "main.go")
		}
		if _, err := os.Stat(p); err == nil {
			if candidate != "." {
				mainPkgDir = filepath.Join(abs, candidate)
			}
			break
		}
	}

	distDir := filepath.Join(abs, "dist")
	if err := os.MkdirAll(distDir, 0750); err != nil {
		return err
	}
	wasmOut := filepath.Join(distDir, "app.wasm")

	fmt.Printf("building (tinygo): . → %s\n", wasmOut)
	args := tinygoBuildArgs(wasmOut, debug, release)

	cmd := exec.Command("tinygo", args...) // #nosec G204 -- args come from tinygoBuildArgs, vane's own build flags, not attacker input; tinygo binary is hardcoded
	cmd.Dir = mainPkgDir
	cmd.Env = tinygoEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tinygo build: %w", err)
	}

	if release && !skipOptimize {
		if err := runWasmOpt(wasmOut); err != nil {
			return err
		}
	}

	// Copy public/ → dist/
	publicDir := filepath.Join(abs, "public")
	if _, err := os.Stat(publicDir); err == nil {
		if err := copyDir(publicDir, distDir); err != nil {
			return fmt.Errorf("copy public: %w", err)
		}
	}

	// Copy co-located .css files
	if err := copyCSSFiles(abs, distDir); err != nil {
		return fmt.Errorf("copy css: %w", err)
	}

	// TinyGo requires its own wasm_exec.js, overwriting the standard Go version.
	if err := copyTinyGoWasmExec(distDir); err != nil {
		return fmt.Errorf("copy tinygo wasm_exec.js: %w", err)
	}

	fmt.Println("done →", distDir)
	return nil
}

// tinygoEnv returns os.Environ() with a Windows-specific fix: when GOROOT is on
// a different drive than LOCALAPPDATA, TinyGo's goroot cache uses cross-drive
// directory junctions that Windows refuses to execute through (CreateProcess
// fails). Redirecting LOCALAPPDATA to the same drive as GOROOT avoids the issue.
func tinygoEnv() []string {
	env := os.Environ()
	if runtime.GOOS != "windows" {
		return env
	}
	localappdata := os.Getenv("LOCALAPPDATA")
	if localappdata == "" {
		return env
	}
	// GOROOT is often not an env var, so query the Go toolchain.
	goroot := os.Getenv("GOROOT")
	if goroot == "" {
		if out, err := exec.Command("go", "env", "GOROOT").Output(); err == nil {
			goroot = strings.TrimSpace(string(out))
		}
	}
	if goroot == "" {
		return env
	}
	gorootVol := filepath.VolumeName(goroot)
	cacheVol := filepath.VolumeName(localappdata)
	if gorootVol == "" || strings.EqualFold(gorootVol, cacheVol) {
		return env
	}
	// Different drives: redirect TinyGo cache to GOROOT's drive.
	newCache := filepath.Join(gorootVol+`\`, "tinygo_cache")
	result := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), "LOCALAPPDATA=") {
			continue
		}
		result = append(result, e)
	}
	return append(result, "LOCALAPPDATA="+newCache)
}

// copyTinyGoWasmExec copies TinyGo's wasm_exec.js from TINYGOROOT into destDir.
func copyTinyGoWasmExec(destDir string) error {
	cmd := exec.Command("tinygo", "env", "TINYGOROOT")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("tinygo not found in PATH: %w", err)
	}
	root := strings.TrimSpace(string(out))
	src := filepath.Join(root, "targets", "wasm_exec.js")
	data, err := os.ReadFile(src) // #nosec G304 -- root comes from the local tinygo toolchain's own `tinygo env` output, not attacker input
	if err != nil {
		return fmt.Errorf("wasm_exec.js not found at %s: %w", src, err)
	}
	return os.WriteFile(filepath.Join(destDir, "wasm_exec.js"), data, 0600) // #nosec G703 -- destDir is the project's own dist/ directory
}
