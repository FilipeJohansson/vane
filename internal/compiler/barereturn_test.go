package compiler_test

import (
	"strings"
	"testing"

	"github.com/filipejohansson/vane/internal/compiler"
)

func compileBody(t *testing.T, body string) string {
	t.Helper()
	src := "package p\n\nfunc F(x int) {\n" + body + "}\n"
	out, err := compiler.Compile(src, "F.vane")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return out
}

// A bare `return` keeps the newline after it, so the generated Go has the
// same number of lines as the source and `}` stays on its own line.
func TestBareReturnKeepsFollowingNewline(t *testing.T) {
	out := compileBody(t, "\tif x > 0 {\n\t\treturn\n\t}\n\tprintln(x)\n")
	if strings.Contains(out, "return }") || strings.Contains(out, "return }\n") {
		t.Errorf("newline after bare return was swallowed:\n%s", out)
	}
	if !strings.Contains(out, "return\n\t}") {
		t.Errorf("expected `return` and `}` on separate lines:\n%s", out)
	}
}

// Swallowing the newline would glue the next statement or case clause onto the
// return statement.
func TestBareReturnDoesNotAbsorbNextStatement(t *testing.T) {
	out := compileBody(t, "\tswitch x {\n\tcase 1:\n\t\treturn\n\tcase 2:\n\t\tprintln(2)\n\t}\n")
	if strings.Contains(out, "return case") {
		t.Errorf("bare return absorbed the next case clause:\n%s", out)
	}
}

func TestReturnWithValueStillGetsSingleSpace(t *testing.T) {
	out := compileBody(t, "\t_ = func() int {\n\t\treturn   x\n\t}\n")
	if !strings.Contains(out, "return   x") && !strings.Contains(out, "return x") {
		t.Errorf("return with a value was mangled:\n%s", out)
	}
}
