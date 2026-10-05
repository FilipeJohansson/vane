package lsp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPathToFileURI_PercentEncodesSpaces(t *testing.T) {
	tests := []struct {
		name, path, want string
	}{
		{"windows drive with spaces", "D:/Users/John Doe/OneDrive - Company/Home.vane", "file:///D:/Users/John%20Doe/OneDrive%20-%20Company/Home.vane"},
		{"windows drive no spaces", "D:/proj/Home.vane", "file:///D:/proj/Home.vane"},
		{"unix with spaces", "/home/john doe/proj/Home.vane", "file:///home/john%20doe/proj/Home.vane"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pathToFileURI(tt.path); got != tt.want {
				t.Errorf("pathToFileURI(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// A path with a space must produce the same store key as the URI VS Code
// sends for that file, or lookups for unopened documents miss.
func TestPathToFileURI_MatchesEditorURIKey(t *testing.T) {
	const editorURI = "file:///d%3A/Users/John%20Doe/Home.vane"
	got := normalizeFileURI(pathToFileURI("D:/Users/John Doe/Home.vane"))
	if want := normalizeFileURI(editorURI); got != want {
		t.Errorf("store key %q != editor key %q", got, want)
	}
}

// A definition result pointing into a .vane file that is not in the store
// must be dropped, not forwarded with untranslated go-coordinates.
func TestTranslateResponsePos_DropsResultForUnknownDoc(t *testing.T) {
	store := newDocStore()
	msg := Message(`{"jsonrpc":"2.0","id":1,"result":[{"uri":"file:///D:/proj/Closed.vane","range":{"start":{"line":40,"character":3},"end":{"line":40,"character":9}}}]}`)

	got := translateResponsePos(msg, store)

	var resp struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(got, &resp); err != nil {
		t.Fatal(err)
	}
	if string(resp.Result) != "null" {
		t.Errorf("result = %s, want null (untranslated range must not be forwarded)", resp.Result)
	}
	if strings.Contains(string(got), `"line":40`) {
		t.Errorf("response still carries the untranslated go-coordinate range: %s", got)
	}
}
