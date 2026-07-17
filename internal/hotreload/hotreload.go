package hotreload

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//* SSE broadcaster

type Hub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[chan string]struct{})}
}

func (h *Hub) subscribe() chan string {
	ch := make(chan string, 1)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsubscribe(ch chan string) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

func (h *Hub) Broadcast(event string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- event:
		default:
		}
	}
}

//* SSE handler

func SSEHandler(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := hub.subscribe()
		defer hub.unsubscribe(ch)

		for {
			select {
			case event := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", event)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	}
}

// SSESnippet is injected before </body> in the served index.html.
// A .go/.vane change triggers a real navigation (location.reload()), which
// drops scroll position along with everything else in memory. sessionStorage
// survives that reload (same tab, same origin), so scroll position is saved
// right before reloading and restored after. Restoring is retried on a timer
// rather than done once, since the WASM app hasn't necessarily re-mounted
// its content (and thus grown the page back to a scrollable height) by the
// time this script runs. setTimeout, not requestAnimationFrame: the editor
// window typically has focus (not the browser tab) at the moment a save
// triggers this reload, and browsers fully suspend rAF callbacks in
// backgrounded tabs, whereas timers still fire (just throttled).
const SSESnippet = `<script>
(function() {
  var es = new EventSource('/__vane_reload');
  es.onmessage = function(e) {
    if (e.data === 'reload') {
      try { sessionStorage.setItem('__vane_scroll', String(window.scrollY)); } catch (_) {}
      location.reload();
      return;
    }
    if (e.data === 'css') {
      document.querySelectorAll('link[rel=stylesheet]').forEach(function(el) {
        var u = new URL(el.href); u.searchParams.set('_r', Date.now()); el.href = u.toString();
      });
    }
  };

  var saved = null;
  try { saved = sessionStorage.getItem('__vane_scroll'); } catch (_) {}
  if (saved !== null) {
    try { sessionStorage.removeItem('__vane_scroll'); } catch (_) {}
    var target = parseFloat(saved);
    var deadline = Date.now() + 8000;
    var restore = function() {
      window.scrollTo(0, target);
      if (Math.abs(window.scrollY - target) <= 1 || Date.now() > deadline) return;
      setTimeout(restore, 10);
    };
    restore();
  }
})();
</script>`

// VaneDebugSnippet is injected before </body> in debug builds.
// It buffers Go panic output (which arrives line-by-line via console.warn)
// into a single styled console.group. Stack frames referencing .vane files
// are highlighted; when /__vane_src/ is available the affected source lines
// are fetched and shown inline.
//
// Note: Go 1.24 js/wasm does not emit DWARF, so browser breakpoints are not
// possible. However, //line directives ARE reflected in the runtime symbol
// table, meaning panic stack traces already reference the original .vane
// file + line, which this snippet surfaces.
const VaneDebugSnippet = `<script>
(function () {
  'use strict';
  var _lines = [];
  var _active = false;
  var _orig = console.warn.bind(console);

  // Match a Go stack file line: "\t<path>.vane:<line> +0x..."
  // path may be http://... (debug) or an absolute OS path (release build).
  function parseVaneFrame(line) {
    var m = line.match(/^\s+(\S+\.vane):(\d+)/);
    return m ? { url: m[1], line: parseInt(m[2], 10) } : null;
  }

  // Fetch .vane source and print a 5-line snippet around the panic line.
  // Only works in debug mode where /__vane_src/ is served.
  function showSourceSnippet(frame) {
    if (!frame.url.startsWith('http')) return; // can't fetch OS paths
    fetch(frame.url)
      .then(function (r) { return r.text(); })
      .then(function (src) {
        var all = src.split('\n');
        var idx = frame.line - 1;
        var lo  = Math.max(0, idx - 2);
        var hi  = Math.min(all.length - 1, idx + 2);
        var out = '';
        for (var i = lo; i <= hi; i++) {
          out += (i === idx ? '→ ' : '  ') + (i + 1) + '\t' + all[i] + '\n';
        }
        _orig('%c' + frame.url + ':' + frame.line, 'color:#f97316;font-weight:bold');
        _orig('%c' + out, 'font-family:monospace;white-space:pre;color:#e2e8f0');
      })
      .catch(function () {});
  }

  function _flush() {
    if (!_lines.length) return;
    var firstVane = null;
    console.group('%c[vane] Go panic', 'color:#ef4444;font-weight:bold');
    _lines.forEach(function (l) {
      var vane = parseVaneFrame(l);
      if (vane) {
        if (!firstVane) firstVane = vane;
        _orig('%c' + l, 'color:#f97316');
      } else {
        _orig(l);
      }
    });
    if (firstVane) showSourceSnippet(firstVane);
    console.groupEnd();
    _lines = [];
    _active = false;
  }

  console.warn = function () {
    var msg = Array.prototype.slice.call(arguments).join(' ');
    if (!_active && !/^(goroutine \d|panic:|runtime error:)/.test(msg)) {
      return _orig.apply(console, arguments);
    }
    _active = true;
    if (msg !== '') _lines.push(msg);
    if (msg === 'exit status 2') _flush();
  };

  fetch('/__vane_map.json')
    .then(function (r) { return r.json(); })
    .then(function (d) { window.__vaneMap = d; })
    .catch(function () {});
})();
</script>`

//* File watcher

type fileState struct {
	mod  time.Time
	size int64
}

var watchSkipDirs = map[string]bool{"dist": true, ".git": true, "vendor": true}

// Callbacks holds the build/copy functions injected from main to avoid
// circular imports.
type Callbacks struct {
	Build          func(projectDir string) error
	Rebuild        func(projectDir string, changedFiles []string) error // optional; called instead of Build on .go/.vane changes
	OnCSSChange    func(changedFiles []string)                          // optional; called before CSS hot-swap
	OnPublicChange func()                                               // optional; called before public reload
	CopyDir        func(src, dst string) error
	CopyCSSFiles   func(projectDir, distDir string) error
}

func Watch(projectDir, distDir string, hub *Hub, cb Callbacks) {
	state := make(map[string]fileState)

	// Initial snapshot, no triggers.
	_ = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, _ error) error {
		if d.IsDir() {
			if watchSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if info, err := d.Info(); err == nil && isWatchedExt(path) {
			state[path] = fileState{mod: info.ModTime(), size: info.Size()}
		}
		return nil
	})

	for {
		time.Sleep(300 * time.Millisecond)

		var goChanged, cssChanged, publicChanged bool
		var changedGoFiles, changedCSSFiles []string
		seen := make(map[string]struct{}, len(state))

		_ = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, _ error) error {
			if d.IsDir() {
				if watchSkipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if !isWatchedExt(path) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			seen[path] = struct{}{}
			cur := fileState{mod: info.ModTime(), size: info.Size()}
			prev, known := state[path]
			if !known || prev != cur {
				state[path] = cur
				if known {
					classifyChange(projectDir, path, &goChanged, &cssChanged, &publicChanged, &changedGoFiles, &changedCSSFiles)
				}
			}
			return nil
		})

		// Detect deletions.
		for path := range state {
			if _, ok := seen[path]; !ok {
				delete(state, path)
				classifyChange(projectDir, path, &goChanged, &cssChanged, &publicChanged, &changedGoFiles, &changedCSSFiles)
			}
		}

		switch {
		case goChanged:
			var buildErr error
			if cb.Rebuild != nil {
				buildErr = cb.Rebuild(projectDir, changedGoFiles)
			} else {
				fmt.Println("\n[vane] rebuilding...")
				buildErr = cb.Build(projectDir)
			}
			if buildErr != nil {
				if msg := buildErr.Error(); msg != "" {
					fmt.Fprintln(os.Stderr, "[vane] build error:", msg)
				}
			} else {
				hub.Broadcast("reload")
			}
		case publicChanged:
			if cb.OnPublicChange != nil {
				cb.OnPublicChange()
			} else {
				fmt.Println("[vane] public changed, reloading...")
			}
			_ = cb.CopyDir(filepath.Join(projectDir, "public"), distDir)
			hub.Broadcast("reload")
		case cssChanged:
			if cb.OnCSSChange != nil {
				cb.OnCSSChange(changedCSSFiles)
			} else {
				fmt.Println("[vane] css changed, hot-swapping...")
			}
			_ = cb.CopyCSSFiles(projectDir, distDir)
			_ = cb.CopyDir(filepath.Join(projectDir, "public"), distDir)
			hub.Broadcast("css")
		}
	}
}

func classifyChange(projectDir, path string, goChanged, cssChanged, publicChanged *bool, changedGoFiles, changedCSSFiles *[]string) {
	rel, _ := filepath.Rel(projectDir, path)
	rel = filepath.ToSlash(rel)
	ext := filepath.Ext(path)

	switch {
	case ext == ".go" || ext == ".vane":
		if strings.HasSuffix(path, "_vane.go") {
			return // LSP-generated file, ignore
		}
		*goChanged = true
		*changedGoFiles = append(*changedGoFiles, filepath.Base(path))
	case ext == ".css":
		*cssChanged = true
		*changedCSSFiles = append(*changedCSSFiles, filepath.Base(path))
	case strings.HasPrefix(rel, "public/"):
		*publicChanged = true
	}
}

func isWatchedExt(path string) bool {
	if strings.HasSuffix(path, "_vane.go") {
		return false // LSP-generated file, ignore
	}
	switch filepath.Ext(path) {
	case ".go", ".vane", ".css", ".html", ".js":
		return true
	}
	return false
}
