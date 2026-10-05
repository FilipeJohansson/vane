package lsp

import (
	"encoding/json"
	"strings"
	"testing"
)

func syncCapability(t *testing.T, in string) map[string]json.RawMessage {
	t.Helper()
	out := stripExecuteCommandProvider(Message(in))
	var resp struct {
		Result struct {
			Capabilities map[string]json.RawMessage `json:"capabilities"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatal(err)
	}
	var sync map[string]json.RawMessage
	if err := json.Unmarshal(resp.Result.Capabilities["textDocumentSync"], &sync); err != nil {
		t.Fatalf("textDocumentSync is not an options object: %s", resp.Result.Capabilities["textDocumentSync"])
	}
	return sync
}

// The editor only sends didOpen/didClose when openClose is advertised, and the
// proxy relies on them to know which documents are open.
func TestStripExecuteCommandProvider_KeepsOpenCloseAndOtherSyncFields(t *testing.T) {
	sync := syncCapability(t, `{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"textDocumentSync":{"openClose":true,"change":2,"save":{}}}}}`)
	if string(sync["openClose"]) != "true" {
		t.Errorf("openClose = %s, want true", sync["openClose"])
	}
	if string(sync["change"]) != "1" {
		t.Errorf("change = %s, want 1 (full sync)", sync["change"])
	}
	if _, ok := sync["save"]; !ok {
		t.Errorf("save option was dropped: %v", sync)
	}
}

func TestStripExecuteCommandProvider_EnablesOpenCloseForBareSyncKind(t *testing.T) {
	sync := syncCapability(t, `{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"textDocumentSync":2}}}`)
	if string(sync["openClose"]) != "true" {
		t.Errorf("openClose = %s, want true", sync["openClose"])
	}
	if string(sync["change"]) != "1" {
		t.Errorf("change = %s, want 1 (full sync)", sync["change"])
	}
}

func TestStripExecuteCommandProvider_StillRemovesExecuteCommandProvider(t *testing.T) {
	out := string(stripExecuteCommandProvider(Message(`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"executeCommandProvider":{"commands":["gopls.x"]},"textDocumentSync":{"change":2}}}}`)))
	if !json.Valid([]byte(out)) || strings.Contains(out, "executeCommandProvider") {
		t.Errorf("executeCommandProvider not removed: %s", out)
	}
}
