package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// goldenDir holds one golden file per public package, relative to the repo
// root.
const goldenDir = "goldens"

func main() {
	write := flag.Bool("write", false, "regenerate the golden files instead of checking them")
	flag.Parse()

	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "apisurface:", err)
		os.Exit(1)
	}

	surfaces, err := Dump(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "apisurface:", err)
		os.Exit(1)
	}

	if *write {
		writeGoldens(repoRoot, surfaces)
		return
	}

	checkGoldens(repoRoot, surfaces)
}

func writeGoldens(repoRoot string, surfaces []Surface) {
	dir := filepath.Join(repoRoot, goldenDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "apisurface:", err)
		os.Exit(1)
	}
	for _, s := range surfaces {
		path := filepath.Join(dir, s.Package.Golden)
		if err := os.WriteFile(path, []byte(s.Content), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "apisurface:", err)
			os.Exit(1)
		}
		fmt.Println("apisurface: wrote", filepath.Join(goldenDir, s.Package.Golden))
	}
}

func checkGoldens(repoRoot string, surfaces []Surface) {
	mismatch := false
	for _, s := range surfaces {
		rel := filepath.Join(goldenDir, s.Package.Golden)
		path := filepath.Join(repoRoot, rel)

		want, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "apisurface:", rel, "does not exist yet")
			mismatch = true
			continue
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "apisurface: reading %s: %v\n", rel, err)
			mismatch = true
			continue
		}
		if s.Content != string(want) {
			fmt.Fprintln(os.Stderr, "apisurface:", s.Package.ImportPath, "does not match", rel)
			mismatch = true
		}
	}

	if mismatch {
		fmt.Fprintln(os.Stderr, "apisurface: if this change is intentional, run:")
		fmt.Fprintln(os.Stderr, "apisurface:   go run ./internal/apisurface -write")
		fmt.Fprintln(os.Stderr, "apisurface: and commit the updated golden file(s)")
		os.Exit(1)
	}

	fmt.Println("apisurface: OK, all golden files up to date")
}
