package lsp

import (
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingClosers(t *testing.T) {
	tests := []struct {
		name, line, want string
	}{
		{"balanced", `{router.Link(a, b)}`, ""},
		{"unclosed call and block", `{router.Link(router.LinkProps{  }`, ")}"},
		{"nested innermost first", `{items.Map(func() {`, "})}"},
		{"bracket inside string ignored", `{fmt.Sprint("(")`, "}"},
		{"bracket inside raw string ignored", "{fmt.Sprint(`(`)", "}"},
		{"bracket in trailing comment ignored", `{call( // (`, ")}"},
		{"apostrophe in text is not a rune literal", `<p>Here's {x(`, ")}"},
		{"mismatched closer is unrepairable", `{a(]`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := missingClosers(tt.line); got != tt.want {
				t.Errorf("missingClosers(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestRepairEditedLine(t *testing.T) {
	prev := "a\nb\n{router.Link(x)}\nc"
	text := "a\nb\n{router.Link(router.LinkProps{  }\nc"
	got, ok := repairEditedLine(prev, text)
	if !ok {
		t.Fatal("expected a repair")
	}
	if want := "a\nb\n{router.Link(router.LinkProps{  })}\nc"; got != want {
		t.Errorf("repaired = %q, want %q", got, want)
	}
	if _, ok := repairEditedLine(prev, prev); ok {
		t.Error("unchanged text must not be repaired")
	}
	if _, ok := repairEditedLine(prev, "a\nb\n{router.Link(x)}\nc ok"); ok {
		t.Error("a balanced edited line has nothing to close")
	}
}

func TestRepairEditedLine_KeepsCRLF(t *testing.T) {
	got, ok := repairEditedLine("a\r\nb(\r\nc", "a\r\nb(x\r\nc")
	if !ok || got != "a\r\nb(x)\r\nc" {
		t.Errorf("got %q ok=%v", got, ok)
	}
}

// While a bracket is still open the file does not compile; the server must keep
// feeding gopls the edited text (with the edited line closed) instead of
// staying on the last version that compiled.
func TestHandleDidChange_UnclosedBracketStillUpdatesGeneratedGo(t *testing.T) {
	vaneURI := pathToFileURI(filepath.Join(t.TempDir(), "Test.vane"))
	good := blockCompletionSrc
	broken := strings.Replace(good, `{router.Link(router.LinkProps{  }, "x")}`, `{router.Link(router.LinkProps{  }`, 1)

	store := newTestStore(vaneURI, mustDoc(t, good))
	if _, _, err := store.compile(vaneURI, broken); err == nil {
		t.Fatal("test premise: the unrepaired text should not compile")
	}

	change, _ := json.Marshal(map[string]any{
		"method": "textDocument/didChange",
		"params": map[string]any{
			"textDocument":   map[string]any{"uri": vaneURI, "version": 2},
			"contentChanges": []map[string]any{{"text": broken}},
		},
	})
	if !handleDidChange(Message(change), io.Discard, store) {
		t.Fatal("didChange not handled")
	}

	doc, ok := store.get(vaneURI)
	if !ok {
		t.Fatal("document missing from store")
	}
	if got, _ := store.text(vaneURI); got != broken {
		t.Errorf("store must keep the text as typed, got %q", got)
	}
	if !strings.Contains(strings.Join(doc.goLines, "\n"), `router.Link(router.LinkProps{  })`) {
		t.Errorf("generated Go was not rebuilt from the repaired text:\n%s", strings.Join(doc.goLines, "\n"))
	}
}
