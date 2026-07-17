//go:build js && wasm

package core_test

import (
	"testing"

	"github.com/filipejohansson/vane/core"
)

// TestIsRelease_FalseWithoutReleaseLdflag verifies the default (non-release)
// state: test binaries are never built with -ldflags -X
// core.releaseFlag=1, so IsRelease must report false. The true branch is
// exercised for real by vane build --release / --tinygo --release, verified
// against wasmBuildArgs/tinygoBuildArgs at the CLI level instead, since
// releaseFlag can only be set at link time, not from within a test.
func TestIsRelease_FalseWithoutReleaseLdflag(t *testing.T) {
	if core.IsRelease() {
		t.Error("IsRelease() = true in a normal test build, want false")
	}
}
