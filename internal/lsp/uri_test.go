package lsp

import (
	"strings"
	"testing"
)

// TestVaneToVirtual_URIRewrite and its counterpart below cover the .vane <->
// _vane.go URI rewrite that lets gopls, which never heard of .vane files,
// operate on the compiled overlay.
func TestVaneToVirtual_URIRewrite(t *testing.T) {
	msg := Message(`{"method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///D:/proj/Home.vane","languageId":"go"}}}`)
	got := string(vaneToVirtual(msg))
	if !strings.Contains(got, `"uri":"file:///D:/proj/Home_vane.go"`) {
		t.Errorf("vaneToVirtual didn't rewrite .vane -> _vane.go: %s", got)
	}
	if strings.Contains(got, ".vane\"") {
		t.Errorf("vaneToVirtual left a .vane URI in place: %s", got)
	}
}

func TestVirtualToVane_URIRewrite(t *testing.T) {
	msg := Message(`{"uri":"file:///D:/proj/Home_vane.go","diagnostics":[]}`)
	got := string(virtualToVane(msg))
	if !strings.Contains(got, `"uri":"file:///D:/proj/Home.vane"`) {
		t.Errorf("virtualToVane didn't rewrite _vane.go -> .vane: %s", got)
	}
}

func TestVaneToVirtual_VirtualToVane_RoundTrip(t *testing.T) {
	original := Message(`{"uri":"file:///D:/proj/Home.vane","text":"<div>hi</div>"}`)
	roundTripped := virtualToVane(vaneToVirtual(original))
	if string(roundTripped) != string(original) {
		t.Errorf("round trip changed the message: got %s, want %s", roundTripped, original)
	}
}

// TestNormalizeURIs_LowercasePercentEncodedDriveLetter is VS Code on
// Windows's actual URI shape (file:///d%3A/proj/Home.vane, lowercase drive
// letter + percent-encoded colon), which must normalize to what gopls uses
// (file:///D:/proj/Home.vane) or gopls can't match the file to any
// workspace view at all.
func TestNormalizeURIs_LowercasePercentEncodedDriveLetter(t *testing.T) {
	msg := Message(`{"uri":"file:///d%3A/proj/Home_vane.go"}`)
	got := string(normalizeURIs(msg))
	want := `{"uri":"file:///D:/proj/Home_vane.go"}`
	if got != want {
		t.Errorf("normalizeURIs(%s) = %s, want %s", msg, got, want)
	}
}
