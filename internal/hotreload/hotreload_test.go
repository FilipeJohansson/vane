package hotreload

import (
	"testing"
	"time"
)

func TestHubBroadcastToSubscriber(t *testing.T) {
	h := NewHub()
	ch := h.subscribe()
	defer h.unsubscribe(ch)

	h.Broadcast("reload")

	select {
	case got := <-ch:
		if got != "reload" {
			t.Errorf("got %q, want %q", got, "reload")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast")
	}
}

func TestHubUnsubscribeStopsDelivery(t *testing.T) {
	h := NewHub()
	ch := h.subscribe()
	h.unsubscribe(ch)

	h.Broadcast("reload")

	select {
	case got, ok := <-ch:
		if ok {
			t.Errorf("unsubscribed channel received %q, want no delivery", got)
		}
	case <-time.After(50 * time.Millisecond):
		// no delivery within the window, as expected
	}
}

func TestHubBroadcastDoesNotBlockOnFullBuffer(t *testing.T) {
	h := NewHub()
	ch := h.subscribe() // buffered size 1
	defer h.unsubscribe(ch)

	done := make(chan struct{})
	go func() {
		// Two broadcasts with no reader draining in between. The second
		// must not block (Hub.Broadcast uses a non-blocking select).
		h.Broadcast("reload")
		h.Broadcast("css")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Broadcast blocked on a full subscriber buffer")
	}
}

func TestHubMultipleSubscribers(t *testing.T) {
	h := NewHub()
	ch1 := h.subscribe()
	ch2 := h.subscribe()
	defer h.unsubscribe(ch1)
	defer h.unsubscribe(ch2)

	h.Broadcast("reload")

	for i, ch := range []chan string{ch1, ch2} {
		select {
		case got := <-ch:
			if got != "reload" {
				t.Errorf("subscriber %d: got %q, want %q", i, got, "reload")
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out waiting for broadcast", i)
		}
	}
}

func TestIsWatchedExt(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"App.go", true},
		{"Card.vane", true},
		{"style.css", true},
		{"index.html", true},
		{"bundle.js", true},
		{"README.md", false},
		{"app.wasm", false},
		{"data.json", false},
		// LSP-generated files must be ignored regardless of extension, or
		// the LSP shim writing these on every keystroke would trigger
		// a full rebuild loop (see internal_docs conventions / prior bug).
		{"Signup_vane.go", false},
		{"src/components/Button_vane.go", false},
	}
	for _, c := range cases {
		if got := isWatchedExt(c.path); got != c.want {
			t.Errorf("isWatchedExt(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestClassifyChange(t *testing.T) {
	const projectDir = "/project"

	t.Run("vane file marks goChanged", func(t *testing.T) {
		var goChanged, cssChanged, publicChanged bool
		var goFiles, cssFiles []string
		classifyChange(projectDir, "/project/src/App.vane", &goChanged, &cssChanged, &publicChanged, &goFiles, &cssFiles)
		if !goChanged {
			t.Error("goChanged = false, want true")
		}
		if len(goFiles) != 1 || goFiles[0] != "App.vane" {
			t.Errorf("goFiles = %v, want [App.vane]", goFiles)
		}
	})

	t.Run("go file marks goChanged", func(t *testing.T) {
		var goChanged, cssChanged, publicChanged bool
		var goFiles, cssFiles []string
		classifyChange(projectDir, "/project/main.go", &goChanged, &cssChanged, &publicChanged, &goFiles, &cssFiles)
		if !goChanged {
			t.Error("goChanged = false, want true")
		}
	})

	t.Run("LSP-generated _vane.go file is ignored", func(t *testing.T) {
		var goChanged, cssChanged, publicChanged bool
		var goFiles, cssFiles []string
		classifyChange(projectDir, "/project/src/Signup_vane.go", &goChanged, &cssChanged, &publicChanged, &goFiles, &cssFiles)
		if goChanged {
			t.Error("goChanged = true for _vane.go file, want false (LSP-generated, should not trigger rebuild)")
		}
		if len(goFiles) != 0 {
			t.Errorf("goFiles = %v, want empty", goFiles)
		}
	})

	t.Run("css file marks cssChanged", func(t *testing.T) {
		var goChanged, cssChanged, publicChanged bool
		var goFiles, cssFiles []string
		classifyChange(projectDir, "/project/src/Button.css", &goChanged, &cssChanged, &publicChanged, &goFiles, &cssFiles)
		if !cssChanged {
			t.Error("cssChanged = false, want true")
		}
		if len(cssFiles) != 1 || cssFiles[0] != "Button.css" {
			t.Errorf("cssFiles = %v, want [Button.css]", cssFiles)
		}
	})

	t.Run("public/ file marks publicChanged", func(t *testing.T) {
		var goChanged, cssChanged, publicChanged bool
		var goFiles, cssFiles []string
		classifyChange(projectDir, "/project/public/index.html", &goChanged, &cssChanged, &publicChanged, &goFiles, &cssFiles)
		if !publicChanged {
			t.Error("publicChanged = false, want true")
		}
	})

	t.Run("unrelated extension is a no-op", func(t *testing.T) {
		var goChanged, cssChanged, publicChanged bool
		var goFiles, cssFiles []string
		classifyChange(projectDir, "/project/README.md", &goChanged, &cssChanged, &publicChanged, &goFiles, &cssFiles)
		if goChanged || cssChanged || publicChanged {
			t.Error("expected no flags set for an unrelated file extension")
		}
	})
}
