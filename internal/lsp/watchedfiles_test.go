package lsp

import (
	"encoding/json"
	"testing"
)

// TestDropRedundantVaneGoChanges_AllRedundant covers the root cause behind
// gopls's "column is beyond end of line" completion failures: VS Code's file
// watcher echoes back every _vane.go write we make (Created/Changed) as a
// workspace/didChangeWatchedFiles notification, on top of the synthetic
// overlay didChange we already sent gopls for the same content. Forwarding
// that echo raced gopls's snapshot invalidation against in-flight completion
// requests. When every entry is such an echo, nothing should be forwarded.
func TestDropRedundantVaneGoChanges_AllRedundant(t *testing.T) {
	msg, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "workspace/didChangeWatchedFiles",
		"params": map[string]any{
			"changes": []map[string]any{
				{"uri": "file:///D:/proj/Modal_vane.go", "type": 2},
				{"uri": "file:///D:/proj/Modal_vane.go", "type": 1},
			},
		},
	})

	_, keep := dropRedundantVaneGoChanges(Message(msg))
	if keep {
		t.Fatalf("expected keep=false when every entry is a redundant _vane.go echo")
	}
}

// TestDropRedundantVaneGoChanges_KeepsOthers verifies a _vane.go delete and an
// unrelated .vane entry both survive filtering, while the redundant _vane.go
// Changed entry alongside them is dropped.
func TestDropRedundantVaneGoChanges_KeepsOthers(t *testing.T) {
	msg, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "workspace/didChangeWatchedFiles",
		"params": map[string]any{
			"changes": []map[string]any{
				{"uri": "file:///D:/proj/Modal_vane.go", "type": 2}, // redundant echo, drop
				{"uri": "file:///D:/proj/Modal_vane.go", "type": 3}, // deleted, keep
				{"uri": "file:///D:/proj/Other.vane", "type": 2},    // .vane, keep
			},
		},
	})

	filtered, keep := dropRedundantVaneGoChanges(Message(msg))
	if !keep {
		t.Fatalf("expected keep=true, non-redundant entries remain")
	}

	var out struct {
		Params struct {
			Changes []struct {
				URI  string `json:"uri"`
				Type int    `json:"type"`
			} `json:"changes"`
		} `json:"params"`
	}
	if err := json.Unmarshal(filtered, &out); err != nil {
		t.Fatalf("unmarshal filtered message: %v", err)
	}
	if len(out.Params.Changes) != 2 {
		t.Fatalf("expected 2 surviving changes, got %d: %+v", len(out.Params.Changes), out.Params.Changes)
	}
	for _, ch := range out.Params.Changes {
		if ch.URI == "file:///D:/proj/Modal_vane.go" && ch.Type == 2 {
			t.Errorf("redundant _vane.go Changed entry should have been dropped")
		}
	}
}

// TestDropRedundantVaneGoChanges_NoneRedundant verifies a notification with no
// _vane.go Created/Changed entries passes through untouched.
func TestDropRedundantVaneGoChanges_NoneRedundant(t *testing.T) {
	msg, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "workspace/didChangeWatchedFiles",
		"params": map[string]any{
			"changes": []map[string]any{
				{"uri": "file:///D:/proj/Other.vane", "type": 2},
			},
		},
	})

	filtered, keep := dropRedundantVaneGoChanges(Message(msg))
	if !keep {
		t.Fatalf("expected keep=true")
	}
	if string(filtered) != string(msg) {
		t.Errorf("expected message to pass through unchanged, got %s", filtered)
	}
}
