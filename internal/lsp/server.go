// Package lsp implements a transparent LSP proxy that sits between the editor
// and gopls, enabling future interception for .vane position translation.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Serve starts the LSP proxy: reads from editorIn, writes to editorOut,
// and forwards all messages transparently to a gopls subprocess.
// Blocks until either side closes.
func Serve(editorIn io.Reader, editorOut io.Writer) error {
	gopls, err := startGopls()
	if err != nil {
		return fmt.Errorf("starting gopls: %w", err)
	}

	goplsIn, err := gopls.StdinPipe()
	if err != nil {
		return fmt.Errorf("gopls stdin: %w", err)
	}
	goplsOut, err := gopls.StdoutPipe()
	if err != nil {
		return fmt.Errorf("gopls stdout: %w", err)
	}
	gopls.Stderr = os.Stderr

	if err := gopls.Start(); err != nil {
		return fmt.Errorf("gopls start: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[vane lsp] gopls started pid=%d\n", gopls.Process.Pid)
	defer func() { _ = gopls.Process.Kill() }()

	store := newDocStore()

	// pendingMethods tracks outgoing request ID → {method, vaneURI} for response translation.
	var pendingMu sync.Mutex
	pendingMethods := make(map[string]pendingInfo)

	var wg sync.WaitGroup
	wg.Add(2)

	// editor → gopls (with interception)
	go func() {
		defer wg.Done()
		proxyEditorToGopls(bufio.NewReader(editorIn), goplsIn, editorOut, store, &pendingMu, pendingMethods)
		_ = goplsIn.Close()
	}()

	// gopls → editor: translate virtual URIs back
	go func() {
		defer wg.Done()
		proxy(bufio.NewReader(goplsOut), editorOut, func(msg Message) Message {
			// Translate hover range from go-coordinates to vane-coordinates.
			{
				var resp struct {
					ID     json.RawMessage `json:"id"`
					Result json.RawMessage `json:"result"`
				}
				if json.Unmarshal(msg, &resp) == nil && resp.ID != nil {
					idStr := string(resp.ID)
					pendingMu.Lock()
					info := pendingMethods[idStr]
					delete(pendingMethods, idStr)
					pendingMu.Unlock()
					switch info.method {
					case "textDocument/hover":
						if len(resp.Result) > 0 && string(resp.Result) != "null" {
							var hoverResult struct {
								Contents struct {
									Kind  string `json:"kind"`
									Value string `json:"value"`
								} `json:"contents"`
								Range *lspRange `json:"range,omitempty"`
							}
							if json.Unmarshal(resp.Result, &hoverResult) == nil {
								if hoverResult.Range != nil {
									origRange := *hoverResult.Range
									if info.vaneURI != "" {
										translated, ok, _ := translateGoRangeToVane(info.vaneURI, origRange, store)
										if ok {
											hoverResult.Range = &translated
										} else {
											hoverResult.Range = nil
										}
									} else {
										hoverResult.Range = nil
									}
								}

								if newResult, err := json.Marshal(hoverResult); err == nil {
									var full map[string]json.RawMessage
									if json.Unmarshal(msg, &full) == nil {
										full["result"] = json.RawMessage(newResult)
										if rebuilt, err2 := json.Marshal(full); err2 == nil {
											msg = Message(rebuilt)
										}
									}
								}
							}
						}

					case "textDocument/completion":
						// gopls returns each item's textEdit range in go-coordinates. The
						// vscode-languageclient rejects/discards completion items whose
						// range doesn't contain the (vane-coordinate) cursor position it
						// asked about, which is why completions never surfaced at all,
						// not just at wrong positions. Translate every range back to vane.
						if len(resp.Result) > 0 && string(resp.Result) != "null" && info.vaneURI != "" {
							if newResult, changed := translateCompletionResultJSON(info.vaneURI, resp.Result, store); changed {
								var full map[string]json.RawMessage
								if json.Unmarshal(msg, &full) == nil {
									full["result"] = newResult
									if rebuilt, err2 := json.Marshal(full); err2 == nil {
										msg = Message(rebuilt)
									}
								}
							}
						}

					case "textDocument/codeAction":
						// Each CodeAction in the response can carry a directly-embedded
						// WorkspaceEdit (e.g. gopls's "organize imports" quick fix) in
						// go-coordinates. This was never translated at all, so the editor
						// applied it verbatim against the .vane buffer, go-coordinates
						// landing at the wrong byte offset in a similarly-shaped but
						// different file corrupts the text (confirmed live: an import-fix
						// edit spliced garbage into the middle of an existing import
						// string literal). Translate every edit and diagnostic range back
						// to vane coordinates before the editor ever sees them.
						if len(resp.Result) > 0 && string(resp.Result) != "null" && info.vaneURI != "" {
							if newResult, changed := translateCodeActionResultJSON(info.vaneURI, resp.Result, store); changed {
								var full map[string]json.RawMessage
								if json.Unmarshal(msg, &full) == nil {
									full["result"] = newResult
									if rebuilt, err2 := json.Marshal(full); err2 == nil {
										msg = Message(rebuilt)
									}
								}
							}
						}
					}
				}
			}
			// Drop cancelled-request error responses. gopls returns "context canceled"
			// when VS Code sends $/cancelRequest before hover/completion completes,
			// and forwarding those causes VS Code to show an error tooltip unnecessarily.
			if isCancelledResponse(msg) {
				return nil
			}
			// Strip executeCommandProvider from initialize response, since gopls
			// advertises gopls.* commands there too, causing duplicate registration
			// when the Go extension's own gopls later tries to register the same commands.
			msg = stripExecuteCommandProvider(msg)
			msg = virtualToVane(msg)
			msg = translateApplyEdit(msg, store)
			msg = translateDiagnosticsPos(msg, store)
			return translateResponsePos(msg, store)
		})
	}()

	// watch for gopls exit
	go func() {
		_ = gopls.Wait()
	}()

	fmt.Fprintf(os.Stderr, "[vane lsp] proxy ready, waiting for editor\n")
	wg.Wait()
	return nil
}

// pendingInfo holds per-request state for the gopls response handler.
type pendingInfo struct {
	method  string
	vaneURI string // normalized vane URI, set for hover requests
}

// rpcMsg is a minimal JSONRPC message envelope for method/id extraction.
type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
}

// didOpenMsg is the full JSONRPC message shape for textDocument/didOpen.
type didOpenMsg struct {
	Params struct {
		TextDocument struct {
			URI        string `json:"uri"`
			LanguageID string `json:"languageId"`
			Version    int    `json:"version"`
			Text       string `json:"text"`
		} `json:"textDocument"`
	} `json:"params"`
}

// didChangeMsg is the full JSONRPC message shape for textDocument/didChange.
type didChangeMsg struct {
	Params struct {
		TextDocument struct {
			URI     string `json:"uri"`
			Version int    `json:"version"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
	} `json:"params"`
}

// didCloseMsg is the full JSONRPC message shape for textDocument/didClose.
type didCloseMsg struct {
	Params struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	} `json:"params"`
}

// proxyEditorToGopls forwards editor→gopls messages with interception for:
//   - shutdown/exit: handled locally so gopls stays alive
//   - textDocument/didOpen,didChange,didClose for .vane files: compile and
//     send synthetic virtual Go document to gopls instead
func proxyEditorToGopls(src *bufio.Reader, goplsIn io.Writer, editorOut io.Writer, store *docStore, pendingMu *sync.Mutex, pendingMethods map[string]pendingInfo) {
	var rootURI string
	for {
		msg, err := ReadMessage(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[vane lsp] editor read error: %v\n", err)
			return
		}

		var env rpcMsg
		if jsonErr := json.Unmarshal(msg, &env); jsonErr == nil {
			switch env.Method {

			case "initialize":
				var initParams struct {
					Params struct {
						RootURI string `json:"rootUri"`
					} `json:"params"`
				}
				if json.Unmarshal(msg, &initParams) == nil {
					rootURI = initParams.Params.RootURI
				}

			case "initialized":
				// Precompile all .vane files to disk before forwarding "initialized" so
				// gopls's packages.Load (triggered by receiving "initialized") finds the
				// generated files. Overlays sent after packages.Load are NOT included in
				// go list package metadata, only disk files are.
				if rootURI != "" {
					precompileWorkspace(uriToPath(rootURI), store)
				}

			case "shutdown":
				if env.ID != nil {
					resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":null}`, env.ID)
					_ = WriteMessage(editorOut, Message(resp))
				}
				continue

			case "exit":
				continue

			case "textDocument/didOpen":
				if handleDidOpen(msg, goplsIn, store) {
					continue // intercepted, don't forward original
				}

			case "textDocument/didChange":
				if handleDidChange(msg, goplsIn, store) {
					continue
				}

			case "textDocument/didClose":
				if handleDidClose(msg, goplsIn, store) {
					continue
				}

			case "workspace/didChangeWatchedFiles":
				handleWatchedFilesChange(msg, goplsIn, store)
				// VS Code's own file watcher notices every _vane.go write we make in
				// response to a .vane didChange (see handleDidChange) and echoes it
				// straight back here. Forwarding that echo to gopls on top of the
				// synthetic overlay didChange we already sent (which is the up-to-date,
				// authoritative copy) creates two concurrent change signals per
				// keystroke, racing gopls's snapshot invalidation against in-flight
				// completion/codeAction requests. Drop the created/changed
				// entries for _vane.go; deletions still need forwarding (handled below).
				if filtered, keep := dropRedundantVaneGoChanges(msg); keep {
					msg = filtered
					// fall through so it still reaches gopls, which needs it to update its index
				} else {
					continue
				}

			case "textDocument/definition", "textDocument/typeDefinition":
				// If the cursor is on a JSX HTML tag name (e.g. <header, </div>),
				// return null immediately, since HTML tags have no definition to navigate to.
				if isJSXTagNamePos(msg, store) {
					if env.ID != nil {
						resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":null}`, env.ID)
						_ = WriteMessage(editorOut, Message(resp))
					}
					continue
				}

			case "textDocument/documentLink":
				// gopls returns pkg.go.dev links for import paths using go-file line
				// numbers. After URI translation to .vane those ranges land on wrong
				// lines (func declarations, JSX) and trigger browser navigation on
				// ctrl+click. Return empty array for all .vane document-link requests.
				if isVaneRequest(msg) {
					if env.ID != nil {
						resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":[]}`, env.ID)
						_ = WriteMessage(editorOut, Message(resp))
					}
					continue
				}
			}
		}

		// Translate position from vane coordinates → Go coordinates before URI swap.
		msg = translateRequestPos(msg, store)
		// Translate any remaining .vane URIs to virtual Go URIs before forwarding.
		msg = vaneToVirtual(msg)

		// Track outgoing requests so the response side can log and translate by method.
		if env.ID != nil && env.Method != "" {
			info := pendingInfo{method: env.Method}
			// For hover, completion, and codeAction, also capture the vane URI (before
			// vaneToVirtual rewrote it) so the response handler can translate returned
			// ranges back to vane coords.
			if env.Method == "textDocument/hover" || env.Method == "textDocument/completion" || env.Method == "textDocument/codeAction" {
				var wrapper struct {
					Params struct {
						TextDocument struct {
							URI string `json:"uri"`
						} `json:"textDocument"`
					} `json:"params"`
				}
				// msg has already been through vaneToVirtual. Parse the original from env context.
				// Re-parse original msg by extracting from the raw message before vaneToVirtual.
				// We stored vaneURI in outer.Params.TextDocument.URI before vaneToVirtual ran.
				// Use a fresh parse against the CURRENT msg (virtual URI now) and reverse-map.
				if json.Unmarshal(msg, &wrapper) == nil {
					vURI := wrapper.Params.TextDocument.URI
					if strings.HasSuffix(vURI, "_vane.go") {
						vaneURI := vURI[:len(vURI)-len("_vane.go")] + ".vane"
						info.vaneURI = normalizeFileURI(vaneURI)
					}
				}
			}
			pendingMu.Lock()
			pendingMethods[string(env.ID)] = info
			pendingMu.Unlock()
		}

		if err := WriteMessage(goplsIn, msg); err != nil {
			fmt.Fprintf(os.Stderr, "[vane lsp] gopls write error: %v\n", err)
			return
		}
	}
}

// vaneToVirtual replaces .vane" occurrences in JSON with _vane.go"
// so gopls receives virtual Go URIs instead of .vane URIs.
func vaneToVirtual(msg Message) Message {
	return normalizeURIs(Message(strings.ReplaceAll(string(msg), `.vane"`, `_vane.go"`)))
}

// normalizeURIs rewrites file URIs in a JSON message so gopls can match them
// to workspace roots. VS Code on Windows sends file:///d%3A/... (lowercase,
// percent-encoded colon) but gopls initializes with file:///D:/ (uppercase,
// unencoded). Without normalization gopls can't find the file in any view.
func normalizeURIs(msg Message) Message {
	s := string(msg)
	s = strings.ReplaceAll(s, `%3A`, `:`)
	s = strings.ReplaceAll(s, `%3a`, `:`)
	return Message(normalizeDriveLetters(s))
}

func normalizeDriveLetters(s string) string {
	const prefix = `file:///`
	if !strings.Contains(s, prefix) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for {
		idx := strings.Index(s, prefix)
		if idx < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:idx+len(prefix)])
		s = s[idx+len(prefix):]
		// s now starts right after "file:///"
		if len(s) >= 2 && s[1] == ':' && s[0] >= 'a' && s[0] <= 'z' {
			b.WriteByte(s[0] - 32)
			s = s[1:]
		}
	}
	return b.String()
}

// virtualToVane is the reverse: _vane.go" → .vane" so the editor sees .vane URIs.
func virtualToVane(msg Message) Message {
	return Message(strings.ReplaceAll(string(msg), `_vane.go"`, `.vane"`))
}

// handleDidOpen intercepts didOpen for .vane files. Returns true if intercepted.
func handleDidOpen(msg Message, goplsIn io.Writer, store *docStore) bool {
	var m didOpenMsg
	if err := json.Unmarshal(msg, &m); err != nil {
		fmt.Fprintf(os.Stderr, "[vane lsp] handleDidOpen unmarshal error: %v\n", err)
		return false
	}
	p := m.Params.TextDocument
	uri := p.URI
	if !isVaneURI(uri) {
		return false
	}

	goContent, sm, err := store.compile(uri, p.Text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[vane lsp] compile error for %s: %v\n", uri, err)
		return true
	}
	store.set(uri, p.Text, goContent, sm)
	writeToDisk(uri, goContent)

	// The _vane.go file exists on disk (written by precompileWorkspace or above),
	// so gopls already knows about it via packages.Load. Send overlay so gopls uses
	// the editor's current content for type-checking (which may differ if user edited
	// the .vane file outside the editor since last session).
	synthetic := buildDidOpen(virtualURI(uri), stripLineDirectives(goContent), p.Version)
	if err := WriteMessage(goplsIn, synthetic); err != nil {
		fmt.Fprintf(os.Stderr, "[vane lsp] send virtual didOpen error: %v\n", err)
	}
	return true
}

// handleDidChange intercepts didChange for .vane files. Returns true if intercepted.
func handleDidChange(msg Message, goplsIn io.Writer, store *docStore) bool {
	var m didChangeMsg
	if err := json.Unmarshal(msg, &m); err != nil {
		return false
	}
	uri := m.Params.TextDocument.URI
	if !isVaneURI(uri) {
		return false
	}
	if len(m.Params.ContentChanges) == 0 {
		return true
	}

	text := m.Params.ContentChanges[0].Text
	goContent, sm, err := store.compile(uri, text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[vane lsp] compile error for %s: %v\n", uri, err)
		return true
	}
	store.set(uri, text, goContent, sm)
	writeToDisk(uri, goContent)

	synthetic := buildDidChange(virtualURI(uri), stripLineDirectives(goContent), m.Params.TextDocument.Version)
	if err := WriteMessage(goplsIn, synthetic); err != nil {
		fmt.Fprintf(os.Stderr, "[vane lsp] send virtual didChange error: %v\n", err)
	}
	return true
}

// handleDidClose intercepts didClose for .vane files. Returns true if intercepted.
func handleDidClose(msg Message, goplsIn io.Writer, store *docStore) bool {
	var m didCloseMsg
	if err := json.Unmarshal(msg, &m); err != nil {
		return false
	}
	uri := m.Params.TextDocument.URI
	if !isVaneURI(uri) {
		return false
	}

	store.delete(m.Params.TextDocument.URI)
	synthetic := buildDidClose(virtualURI(m.Params.TextDocument.URI))
	if err := WriteMessage(goplsIn, synthetic); err != nil {
		fmt.Fprintf(os.Stderr, "[vane lsp] send virtual didClose error: %v\n", err)
	}
	return true
}

func buildDidOpen(uri, text string, version int) Message {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri":        uri,
				"languageId": "go",
				"version":    version,
				"text":       text,
			},
		},
	})
	return Message(body)
}

func buildDidChange(uri, text string, version int) Message {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didChange",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri":     uri,
				"version": version,
			},
			"contentChanges": []map[string]any{
				{"text": text},
			},
		},
	})
	return Message(body)
}

func buildDidClose(uri string) Message {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didClose",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": uri},
		},
	})
	return Message(body)
}

// isCancelledResponse returns true if msg is an LSP error response with
// code -32800 (RequestCancelled) or message "context canceled". These are
// sent by gopls when VS Code cancels a pending request via $/cancelRequest.
func isCancelledResponse(msg Message) bool {
	var r struct {
		ID    json.RawMessage `json:"id"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(msg, &r) != nil || r.Error == nil || r.ID == nil {
		return false
	}
	return r.Error.Code == -32800 || strings.Contains(r.Error.Message, "context canceled")
}

// proxy reads LSP messages from src and writes them to dst.
// If translate is non-nil, each message is transformed before writing.
// translate returning nil drops the message.
func proxy(src *bufio.Reader, dst io.Writer, translate func(Message) Message) {
	for {
		msg, err := ReadMessage(src)
		if err != nil {
			return
		}
		if translate != nil {
			msg = translate(msg)
			if msg == nil {
				continue // dropped
			}
		}
		if err := WriteMessage(dst, msg); err != nil {
			return
		}
	}
}

// stripExecuteCommandProvider removes the executeCommandProvider field from
// the initialize response capabilities, preventing gopls.* command registration
// conflicts with the Go extension's gopls.
func stripExecuteCommandProvider(msg Message) Message {
	var resp struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Result  *struct {
			Capabilities map[string]json.RawMessage `json:"capabilities"`
		} `json:"result,omitempty"`
	}
	if err := json.Unmarshal(msg, &resp); err != nil || resp.Result == nil || len(resp.Result.Capabilities) == 0 {
		return msg
	}
	delete(resp.Result.Capabilities, "executeCommandProvider")

	// Force full-document sync (change=1) instead of incremental (change=2).
	// The vane proxy needs the complete .vane content on every didChange to recompile;
	// incremental deltas cannot be compiled standalone.
	type syncOpts struct {
		Change int `json:"change"`
	}
	var syncVal syncOpts
	if raw, ok := resp.Result.Capabilities["textDocumentSync"]; ok {
		// textDocumentSync may be a number or an object.
		var num int
		if json.Unmarshal(raw, &num) == nil {
			if num != 1 {
				resp.Result.Capabilities["textDocumentSync"] = json.RawMessage(`1`)
			}
		} else if json.Unmarshal(raw, &syncVal) == nil {
			syncVal.Change = 1
			if b, err2 := json.Marshal(syncVal); err2 == nil {
				resp.Result.Capabilities["textDocumentSync"] = json.RawMessage(b)
			}
		}
	}

	filtered, err := json.Marshal(resp)
	if err != nil {
		return msg
	}
	return Message(filtered)
}

// isVaneRequest returns true when the LSP request targets a .vane file.
func isVaneRequest(msg Message) bool {
	var req struct {
		Params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		} `json:"params"`
	}
	if json.Unmarshal(msg, &req) != nil {
		return false
	}
	return isVaneURI(req.Params.TextDocument.URI)
}

// isJSXTagNamePos returns true when the cursor in a .vane file sits on an HTML
// tag name, i.e. the identifier is immediately preceded by '<' or '</'.
// These positions have no meaningful definition to navigate to.
func isJSXTagNamePos(msg Message, store *docStore) bool {
	var req struct {
		Params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Position *lspPos `json:"position,omitempty"`
		} `json:"params"`
	}
	if json.Unmarshal(msg, &req) != nil || req.Params.Position == nil {
		return false
	}
	uri := req.Params.TextDocument.URI
	if !isVaneURI(uri) {
		return false
	}
	doc, ok := store.get(uri)
	if !ok || req.Params.Position.Line >= len(doc.vaneLines) {
		return false
	}
	line := doc.vaneLines[req.Params.Position.Line]
	col := req.Params.Position.Character
	if col < 0 || col > len(line) {
		return false
	}
	// Find start of identifier at col.
	start := col
	if start >= len(line) {
		start = len(line) - 1
	}
	for start > 0 && isIdentByte(line[start-1]) {
		start--
	}
	// Immediately preceded by '<' or '</' ?
	if start > 0 && line[start-1] == '<' {
		return true
	}
	if start > 1 && line[start-2] == '<' && line[start-1] == '/' {
		return true
	}
	return false
}

// isIdentByte reports whether b can appear in a Go/vane identifier.
func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// identAt returns the identifier word that contains column col in line.
func identAt(line string, col int) string {
	if col < 0 || col >= len(line) {
		return ""
	}
	start, end := col, col
	for start > 0 && isIdentByte(line[start-1]) {
		start--
	}
	for end < len(line) && isIdentByte(line[end]) {
		end++
	}
	return line[start:end]
}

// findIdentInLine searches line for ident as a whole word, returning the
// column of the occurrence nearest to preferredCol. Returns -1 if not found.
func findIdentInLine(line, ident string, preferredCol int) int {
	if ident == "" || len(ident) > len(line) {
		return -1
	}
	best := -1
	bestDist := len(line) + 1
	for i := 0; i <= len(line)-len(ident); i++ {
		if line[i:i+len(ident)] != ident {
			continue
		}
		// whole-word boundary
		if i > 0 && isIdentByte(line[i-1]) {
			continue
		}
		if i+len(ident) < len(line) && isIdentByte(line[i+len(ident)]) {
			continue
		}
		dist := i - preferredCol
		if dist < 0 {
			dist = -dist
		}
		// On a tie, prefer the occurrence at or after preferredCol (the "right" one),
		// so that two occurrences equidistant from the hint resolve to the correct one.
		if dist < bestDist || (dist == bestDist && i >= preferredCol && best < preferredCol) {
			bestDist = dist
			best = i
		}
	}
	return best
}

// mapColumn refines the mapped (goLine, gc) by finding the identifier at the vane
// cursor position in the compiled Go output. Returns the corrected (goLine, goCol).
//
// A single vane line can expand to multiple Go lines (e.g. core.El + core.DynProp +
// core.SetProp for one JSX element with attributes). VaneToGo returns the last of
// those lines, but the target identifier (e.g. "step" in step.icon) may live on an
// earlier sibling line (the DynProp line, not the SetProp line). We therefore search
// ALL go lines that carry a //line directive for vaneLine, not just the primary one.
func mapColumn(doc *document, vaneLine, vaneCol, goLine, gc int) (int, int) {
	// Extract the identifier the user is pointing at in the vane source.
	var ident string
	if vaneLine >= 0 && vaneLine < len(doc.vaneLines) {
		ident = identAt(doc.vaneLines[vaneLine], vaneCol)
	}

	tryLine := func(gl int) (int, bool) {
		if gl < 0 || gl >= len(doc.goLines) {
			return 0, false
		}
		s := doc.goLines[gl]
		if s == "" {
			return 0, false
		}
		if ident != "" {
			if col := findIdentInLine(s, ident, gc); col >= 0 {
				return col, true
			}
		}
		return 0, false
	}

	// Try primary go line first.
	if col, ok := tryLine(goLine); ok {
		return goLine, col
	}

	// Try all sibling go lines that share the same vane line mapping.
	if ident != "" && doc.sourceMap != nil {
		for _, gl := range doc.sourceMap.GoLinesForVaneLine(vaneLine) {
			if gl == goLine {
				continue
			}
			if col, ok := tryLine(gl); ok {
				return gl, col
			}
		}
	}

	// Fallback: clamp goLine/gc into valid go-file bounds to prevent gopls
	// erroring "column is beyond end of line" (or worse, an out-of-range line).
	// VaneToGo linearly extrapolates goLine as an offset from the nearest
	// preceding //line directive, which is only accurate in 1:1 passthrough
	// regions; deep inside an expanded/collapsed region (JSX subtrees, nested
	// control flow) that offset can overshoot the real generated file, and
	// ident is often "" here too (cursor mid-typing an as-yet-unmatched
	// prefix like "core.P" for "Portal"), so tryLine never runs.
	if goLine < 0 {
		goLine = 0
	} else if goLine >= len(doc.goLines) {
		goLine = len(doc.goLines) - 1
	}
	if n := len(doc.goLines[goLine]); gc > n {
		gc = n
	} else if gc < 0 {
		gc = 0
	}
	return goLine, gc
}

// translateRequestPos translates position/range fields in requests targeting
// .vane files from vane line coordinates to compiled Go line coordinates.
// Handles both params.position (hover, definition, completion, …) and
// params.range (codeAction, rangeFormatting, …).
// Must be called BEFORE vaneToVirtual because isVaneURI checks for .vane suffix.
func translateRequestPos(msg Message, store *docStore) Message {
	var outer struct {
		Params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Position *lspPos   `json:"position,omitempty"`
			Range    *lspRange `json:"range,omitempty"`
		} `json:"params"`
	}
	if json.Unmarshal(msg, &outer) != nil {
		return msg
	}
	if outer.Params.Position == nil && outer.Params.Range == nil {
		return msg
	}
	vaneURI := outer.Params.TextDocument.URI
	if !isVaneURI(vaneURI) {
		return msg
	}
	doc, ok := store.get(vaneURI)
	if !ok || doc.sourceMap == nil {
		return msg
	}

	var generic map[string]json.RawMessage
	if json.Unmarshal(msg, &generic) != nil {
		return msg
	}
	var params map[string]json.RawMessage
	if json.Unmarshal(generic["params"], &params) != nil {
		return msg
	}

	changed := false

	if outer.Params.Position != nil {
		gl, gc, ok := doc.sourceMap.VaneToGo(outer.Params.Position.Line, outer.Params.Position.Character)
		if !ok {
			// No //line entry covers this vane line at all (e.g. a plain
			// statement before the first directive in the file). Without this
			// fallback the position was left in vane coordinates but still
			// forwarded against the compiled Go content (the URI swap to the
			// virtual Go file happens unconditionally after this function
			// returns), which is exactly the untranslated-position bug behind
			// gopls's "no completions found: column is beyond end of line".
			// Clamp to go line 0, same fallback the Range branch below uses.
			gl, gc = 0, 0
		}
		gl, gc = mapColumn(doc, outer.Params.Position.Line, outer.Params.Position.Character, gl, gc)
		newPos, _ := json.Marshal(lspPos{gl, gc})
		params["position"] = json.RawMessage(newPos)
		changed = true
	}

	if outer.Params.Range != nil {
		sl, sc, sok := doc.sourceMap.VaneToGo(outer.Params.Range.Start.Line, outer.Params.Range.Start.Character)
		el, ec, eok := doc.sourceMap.VaneToGo(outer.Params.Range.End.Line, outer.Params.Range.End.Character)
		if !sok {
			// Fallback: clamp start to go line 0 so gopls gets a valid range.
			sl, sc, sok = 0, 0, true
		}
		if !eok {
			el, ec = sl, sc
			eok = true
		}
		if sok && eok {
			sl, sc = mapColumn(doc, outer.Params.Range.Start.Line, outer.Params.Range.Start.Character, sl, sc)
			el, ec = mapColumn(doc, outer.Params.Range.End.Line, outer.Params.Range.End.Character, el, ec)
			newRange, _ := json.Marshal(lspRange{lspPos{sl, sc}, lspPos{el, ec}})
			params["range"] = json.RawMessage(newRange)
			changed = true
		}
	}

	if !changed {
		return msg
	}
	newParams, _ := json.Marshal(params)
	generic["params"] = json.RawMessage(newParams)
	result, _ := json.Marshal(generic)
	return Message(result)
}

type lspPos struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start lspPos `json:"start"`
	End   lspPos `json:"end"`
}

// translateDiagnosticsPos handles textDocument/publishDiagnostics notifications from
// gopls (no "id", no "result", so not caught by translateResponsePos). Translates every
// diagnostic range from Go coordinates back to .vane coordinates.
func translateDiagnosticsPos(msg Message, store *docStore) Message {
	var notif struct {
		Method string `json:"method"`
		Params struct {
			URI         string `json:"uri"`
			Diagnostics []struct {
				Range    lspRange        `json:"range"`
				Severity int             `json:"severity,omitempty"`
				Code     json.RawMessage `json:"code,omitempty"`
				Source   string          `json:"source,omitempty"`
				Message  string          `json:"message"`
				Tags     json.RawMessage `json:"tags,omitempty"`
			} `json:"diagnostics"`
		} `json:"params"`
	}
	if json.Unmarshal(msg, &notif) != nil || notif.Method != "textDocument/publishDiagnostics" {
		return msg
	}
	uri := notif.Params.URI
	if !isVaneURI(uri) {
		return msg
	}
	doc, ok := store.getByNorm(uri)
	if !ok || doc.sourceMap == nil {
		return msg
	}

	type diagEntry struct {
		Range    lspRange        `json:"range"`
		Severity int             `json:"severity,omitempty"`
		Code     json.RawMessage `json:"code,omitempty"`
		Source   string          `json:"source,omitempty"`
		Message  string          `json:"message"`
		Tags     json.RawMessage `json:"tags,omitempty"`
	}

	var out []diagEntry
	changed := false
	for _, d := range notif.Params.Diagnostics {
		sl, sc, sok := doc.sourceMap.GoToVane(d.Range.Start.Line, d.Range.Start.Character)
		el, ec, eok := doc.sourceMap.GoToVane(d.Range.End.Line, d.Range.End.Character)
		entry := diagEntry{
			Range:    d.Range,
			Severity: d.Severity,
			Code:     d.Code,
			Source:   d.Source,
			Message:  d.Message,
			Tags:     d.Tags,
		}
		if sok && eok {
			entry.Range = lspRange{lspPos{sl, sc}, lspPos{el, ec}}
			changed = true
		} else if sok {
			entry.Range = lspRange{lspPos{sl, sc}, lspPos{sl, sc}}
			changed = true
		}
		out = append(out, entry)
	}
	if !changed {
		return msg
	}

	var generic map[string]json.RawMessage
	if json.Unmarshal(msg, &generic) != nil {
		return msg
	}
	var params map[string]json.RawMessage
	if json.Unmarshal(generic["params"], &params) != nil {
		return msg
	}
	newDiags, _ := json.Marshal(out)
	params["diagnostics"] = json.RawMessage(newDiags)
	newParams, _ := json.Marshal(params)
	generic["params"] = json.RawMessage(newParams)
	result, _ := json.Marshal(generic)
	return Message(result)
}

// translateGoRangeToVane translates a Go-coordinate range to vane coordinates.
// Returns (range, ok, drop): ok=true means translated; drop=true means the
// result lands in the vane preamble (package/import) and should be discarded.
// Used for navigation results (hover, definition, references): the package
// line and import paths have no meaningful "jump to" target in the .vane
// source, so results landing there are suppressed.
func translateGoRangeToVane(uri string, r lspRange, store *docStore) (lspRange, bool, bool) {
	return translateGoRangeToVaneImpl(uri, r, store, true, true)
}

// translateGoRangeToVaneForEdit is like translateGoRangeToVane but never drops
// a range just because it lands in the package/import preamble, and never
// snaps a column to the "containing identifier". Unlike navigation, a
// WorkspaceEdit inserting or replacing text there is directly meaningful: the
// import block is pure passthrough (identical text, no vane-specific
// expansion) between .vane and its compiled .go, so an edit gopls computes
// there, most commonly "add missing import", has a real, correct
// vane-coordinate equivalent. Blanket-dropping it (the right call for
// navigation) silently broke that quick fix instead of just failing safely;
// used by translateWorkspaceEditJSON so codeAction/applyEdit imports work.
func translateGoRangeToVaneForEdit(uri string, r lspRange, store *docStore) (lspRange, bool, bool) {
	return translateGoRangeToVaneImpl(uri, r, store, false, false)
}

func translateGoRangeToVaneImpl(uri string, r lspRange, store *docStore, dropImportBlock, refineColumns bool) (lspRange, bool, bool) {
	doc, ok := store.getByNorm(uri)
	if !ok || doc.sourceMap == nil {
		return r, false, false
	}
	sl, sc, ok := doc.sourceMap.GoToVane(r.Start.Line, r.Start.Character)
	if !ok {
		return r, false, false
	}
	// Drop results that land on the package declaration or import block.
	// Type/const/var/func declarations in the preamble are valid navigation
	// targets; only the package line and import paths should be suppressed
	// (they have no meaningful vane source equivalent), and only when the
	// caller asked for that (dropImportBlock), since edits are the exception.
	inImport := false
	importBlock := make(map[int]bool)
	for i, ln := range doc.vaneLines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "package ") {
			importBlock[i] = true
		} else if t == "import (" || strings.HasPrefix(t, "import (") {
			inImport = true
			importBlock[i] = true
		} else if strings.HasPrefix(t, "import \"") || t == "import" {
			importBlock[i] = true
		} else if inImport {
			importBlock[i] = true
			if t == ")" {
				inImport = false
			}
		}
	}
	if dropImportBlock && importBlock[sl] {
		return r, false, true
	}
	// Refine start column: extract ident from go line, find it in vane line.
	// GoToVane passes the column through unchanged; go and vane lines differ in
	// indentation so the raw column is often off for references/highlights.
	// Only for navigation (refineColumns): edits need the exact column, not
	// "the identifier containing this column", see translateGoRangeToVaneForEdit.
	var goIdent string
	refinedStart := -1
	if refineColumns && r.Start.Line >= 0 && r.Start.Line < len(doc.goLines) && sl < len(doc.vaneLines) {
		goIdent = identAt(doc.goLines[r.Start.Line], r.Start.Character)
		refinedStart = findIdentInLine(doc.vaneLines[sl], goIdent, sc)
		if goIdent != "" && refinedStart >= 0 {
			sc = refinedStart
		}
	}
	el, ec, eok := doc.sourceMap.GoToVane(r.End.Line, r.End.Character)
	if !refineColumns {
		if !eok {
			el, ec = sl, sc
		}
		return lspRange{lspPos{sl, sc}, lspPos{el, ec}}, true, false
	}
	if goIdent != "" && refinedStart >= 0 && r.Start.Line == r.End.Line {
		// Same-line range: set end = refined start + ident length for accurate
		// highlighting. The raw ec from GoToVane has no column refinement.
		el, ec = sl, sc+len(goIdent)
	} else if !eok {
		vaneLineStr := ""
		if sl < len(doc.vaneLines) {
			vaneLineStr = doc.vaneLines[sl]
		}
		el, ec = sl, sc+len(identAt(vaneLineStr, sc))
	}
	return lspRange{lspPos{sl, sc}, lspPos{el, ec}}, true, false
}

// translateCompletionEdit translates the range(s) inside a single edit-shaped
// JSON value from go-coordinates to vane-coordinates. Three shapes are possible
// per the LSP spec: a plain TextEdit ({"range", "newText"}), an
// InsertReplaceEdit ({"insert", "replace", "newText"}), or a bare Range
// ({"start", "end"}), the last is what CompletionList.itemDefaults.editRange
// uses when every item shares the same replacement span. Untranslatable or
// already-vane ranges are left untouched; returns (value, changed).
func translateCompletionEdit(vaneURI string, raw json.RawMessage, store *docStore) (json.RawMessage, bool) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return raw, false
	}
	// translateGoRangeToVaneForEdit, not translateGoRangeToVane: these ranges
	// describe an edit the editor will apply (the completion's own insertion,
	// or itemDefaults.editRange), not a navigation result, so a range landing
	// in the import block, e.g. completing an unimported symbol, which gopls
	// pairs with an edit to add the import must be translated, not dropped.
	if _, isBareRange := obj["start"]; isBareRange {
		var r lspRange
		if json.Unmarshal(raw, &r) != nil {
			return raw, false
		}
		nr, ok, drop := translateGoRangeToVaneForEdit(vaneURI, r, store)
		if !ok || drop {
			return raw, false
		}
		b, _ := json.Marshal(nr)
		return json.RawMessage(b), true
	}
	if rangeRaw, ok := obj["range"]; ok {
		var r lspRange
		if json.Unmarshal(rangeRaw, &r) != nil {
			return raw, false
		}
		nr, ok, drop := translateGoRangeToVaneForEdit(vaneURI, r, store)
		if !ok || drop {
			return raw, false
		}
		b, _ := json.Marshal(nr)
		obj["range"] = json.RawMessage(b)
		out, _ := json.Marshal(obj)
		return json.RawMessage(out), true
	}
	changed := false
	for _, key := range [2]string{"insert", "replace"} {
		raw, ok := obj[key]
		if !ok {
			continue
		}
		var r lspRange
		if json.Unmarshal(raw, &r) != nil {
			continue
		}
		if nr, ok, drop := translateGoRangeToVaneForEdit(vaneURI, r, store); ok && !drop {
			b, _ := json.Marshal(nr)
			obj[key] = json.RawMessage(b)
			changed = true
		}
	}
	if !changed {
		return raw, false
	}
	out, _ := json.Marshal(obj)
	return json.RawMessage(out), true
}

// translateCompletionItemsJSON translates textEdit and additionalTextEdits
// ranges on every completion item from go-coordinates to vane-coordinates.
func translateCompletionItemsJSON(vaneURI string, items []json.RawMessage, store *docStore) ([]json.RawMessage, bool) {
	changed := false
	out := make([]json.RawMessage, len(items))
	for i, raw := range items {
		var item map[string]json.RawMessage
		if json.Unmarshal(raw, &item) != nil {
			out[i] = raw
			continue
		}
		itemChanged := false

		if te, ok := item["textEdit"]; ok && len(te) > 0 && string(te) != "null" {
			if newTE, ok := translateCompletionEdit(vaneURI, te, store); ok {
				item["textEdit"] = newTE
				itemChanged = true
			}
		}

		if ate, ok := item["additionalTextEdits"]; ok && len(ate) > 0 && string(ate) != "null" {
			var edits []map[string]json.RawMessage
			if json.Unmarshal(ate, &edits) == nil {
				edChanged := false
				for j, e := range edits {
					rangeRaw, ok := e["range"]
					if !ok {
						continue
					}
					var r lspRange
					if json.Unmarshal(rangeRaw, &r) != nil {
						continue
					}
					// additionalTextEdits is exactly how gopls adds the import
					// statement when completing an unimported symbol, must
					// translate into the import block, not drop it.
					if nr, ok, drop := translateGoRangeToVaneForEdit(vaneURI, r, store); ok && !drop {
						b, _ := json.Marshal(nr)
						edits[j]["range"] = json.RawMessage(b)
						edChanged = true
					}
				}
				if edChanged {
					b, _ := json.Marshal(edits)
					item["additionalTextEdits"] = json.RawMessage(b)
					itemChanged = true
				}
			}
		}

		if itemChanged {
			b, _ := json.Marshal(item)
			out[i] = json.RawMessage(b)
			changed = true
		} else {
			out[i] = raw
		}
	}
	return out, changed
}

// translateCompletionResultJSON translates a textDocument/completion result
// (either a bare CompletionItem[] or a CompletionList{isIncomplete,items,itemDefaults})
// from go-coordinates to vane-coordinates so the editor's range sanity checks
// (the edit range must contain the cursor position the request was sent for)
// pass and items actually surface, instead of being silently discarded.
func translateCompletionResultJSON(vaneURI string, result json.RawMessage, store *docStore) (json.RawMessage, bool) {
	if len(result) == 0 || string(result) == "null" {
		return result, false
	}

	var list map[string]json.RawMessage
	if json.Unmarshal(result, &list) == nil {
		if itemsRaw, ok := list["items"]; ok {
			var items []json.RawMessage
			if json.Unmarshal(itemsRaw, &items) != nil {
				return result, false
			}
			newItems, changed := translateCompletionItemsJSON(vaneURI, items, store)

			defaultsChanged := false
			if defRaw, ok := list["itemDefaults"]; ok && len(defRaw) > 0 && string(defRaw) != "null" {
				var defaults map[string]json.RawMessage
				if json.Unmarshal(defRaw, &defaults) == nil {
					if er, ok := defaults["editRange"]; ok {
						if newER, ok := translateCompletionEdit(vaneURI, er, store); ok {
							defaults["editRange"] = newER
							defaultsChanged = true
						}
					}
					if defaultsChanged {
						b, _ := json.Marshal(defaults)
						list["itemDefaults"] = json.RawMessage(b)
					}
				}
			}

			if !changed && !defaultsChanged {
				return result, false
			}
			b, _ := json.Marshal(newItems)
			list["items"] = json.RawMessage(b)
			out, _ := json.Marshal(list)
			return json.RawMessage(out), true
		}
	}

	// Bare CompletionItem[] (no isIncomplete/items wrapper).
	var arr []json.RawMessage
	if json.Unmarshal(result, &arr) == nil {
		items, changed := translateCompletionItemsJSON(vaneURI, arr, store)
		if !changed {
			return result, false
		}
		b, _ := json.Marshal(items)
		return json.RawMessage(b), true
	}

	return result, false
}

// translateCodeActionResultJSON translates a textDocument/codeAction response
// ((Command | CodeAction)[]) from go-coordinates to vane-coordinates. A
// CodeAction may carry a directly-embedded WorkspaceEdit (translated via
// translateWorkspaceEditJSON, which itself drops any edit landing in the
// package/import preamble it can't sensibly map) and/or a diagnostics list,
// each with its own range. Plain Command entries (title/command/arguments
// only, no edit/diagnostics) have nothing to translate and pass through
// unchanged. vaneURI is the document the codeAction request targeted, needed
// for diagnostic ranges, which carry no URI of their own.
func translateCodeActionResultJSON(vaneURI string, result json.RawMessage, store *docStore) (json.RawMessage, bool) {
	if len(result) == 0 || string(result) == "null" {
		return result, false
	}
	var rawItems []json.RawMessage
	if json.Unmarshal(result, &rawItems) != nil {
		return result, false
	}

	changed := false
	out := make([]json.RawMessage, len(rawItems))
	for i, raw := range rawItems {
		var item map[string]json.RawMessage
		if json.Unmarshal(raw, &item) != nil {
			out[i] = raw
			continue
		}
		itemChanged := false

		if editRaw, ok := item["edit"]; ok && len(editRaw) > 0 && string(editRaw) != "null" {
			// The edit's own URIs are still in _vane.go virtual form here: this
			// runs before the pipeline's later, unconditional virtualToVane(msg)
			// call, same as the hover/completion translation above it. Apply it
			// locally first so translateWorkspaceEditJSON's isVaneURI checks see
			// the .vane URIs they expect.
			vaned := virtualToVane(Message(editRaw))
			if newEdit, ok2 := translateWorkspaceEditJSON(json.RawMessage(vaned), store); ok2 {
				item["edit"] = newEdit
				itemChanged = true
			} else {
				item["edit"] = json.RawMessage(vaned)
			}
		}

		if diagRaw, ok := item["diagnostics"]; ok && len(diagRaw) > 0 && string(diagRaw) != "null" {
			var diags []map[string]json.RawMessage
			if json.Unmarshal(diagRaw, &diags) == nil {
				diagChanged := false
				for j, d := range diags {
					rangeRaw, ok2 := d["range"]
					if !ok2 {
						continue
					}
					var r lspRange
					if json.Unmarshal(rangeRaw, &r) != nil {
						continue
					}
					if nr, ok3, drop := translateGoRangeToVane(vaneURI, r, store); ok3 && !drop {
						b, _ := json.Marshal(nr)
						diags[j]["range"] = json.RawMessage(b)
						diagChanged = true
					}
				}
				if diagChanged {
					b, _ := json.Marshal(diags)
					item["diagnostics"] = json.RawMessage(b)
					itemChanged = true
				}
			}
		}

		if itemChanged {
			b, _ := json.Marshal(item)
			out[i] = json.RawMessage(b)
			changed = true
		} else {
			out[i] = raw
		}
	}

	if !changed {
		return result, false
	}
	b, _ := json.Marshal(out)
	return json.RawMessage(b), true
}

// translateWorkspaceEditJSON translates all text edit ranges in a WorkspaceEdit
// JSON blob from go-coordinates to vane-coordinates. Handles both `changes` and
// `documentChanges` formats. URIs are expected to already be remapped to .vane
// (virtualToVane has run before this). Returns the translated JSON and whether
// any change was made.
func translateWorkspaceEditJSON(editJSON json.RawMessage, store *docStore) (json.RawMessage, bool) {
	type textEdit struct {
		Range   lspRange        `json:"range"`
		NewText string          `json:"newText"`
		Extra   json.RawMessage `json:"-"`
	}

	// Try documentChanges format first (gopls prefers this).
	var dcWrap struct {
		DocumentChanges []struct {
			TextDocument struct {
				URI     string          `json:"uri"`
				Version json.RawMessage `json:"version,omitempty"`
			} `json:"textDocument"`
			Edits []textEdit `json:"edits"`
		} `json:"documentChanges"`
		Changes json.RawMessage `json:"changes,omitempty"`
	}
	if json.Unmarshal(editJSON, &dcWrap) == nil && len(dcWrap.DocumentChanges) > 0 {
		changed := false
		var keptDCs []struct {
			TextDocument struct {
				URI     string          `json:"uri"`
				Version json.RawMessage `json:"version,omitempty"`
			} `json:"textDocument"`
			Edits []textEdit `json:"edits"`
		}
		for _, tde := range dcWrap.DocumentChanges {
			uri := tde.TextDocument.URI
			if !isVaneURI(uri) {
				keptDCs = append(keptDCs, tde)
				continue
			}
			var keptEdits []textEdit
			for _, edit := range tde.Edits {
				// translateGoRangeToVaneForEdit (not translateGoRangeToVane):
				// edits landing in the import block, e.g. gopls's "add missing
				// import" quick fix, are meaningful and must be translated, not
				// dropped like a navigation result would be.
				r, ok, _ := translateGoRangeToVaneForEdit(uri, edit.Range, store)
				if !ok {
					// No source map coverage at all for this position, drop it
					// entirely rather than leave it with a go-coordinate range
					// that would corrupt the .vane file if the editor applied
					// it verbatim.
					changed = true
					continue
				}
				edit.Range = r
				changed = true
				keptEdits = append(keptEdits, edit)
			}
			tde.Edits = keptEdits
			if len(keptEdits) > 0 {
				keptDCs = append(keptDCs, tde)
			} else {
				changed = true
			}
		}
		if changed {
			dcWrap.DocumentChanges = keptDCs
			b, _ := json.Marshal(dcWrap)
			return json.RawMessage(b), true
		}
		return editJSON, false
	}

	// Try changes format (map URI → []TextEdit).
	var chWrap struct {
		Changes map[string][]textEdit `json:"changes"`
	}
	if json.Unmarshal(editJSON, &chWrap) == nil && len(chWrap.Changes) > 0 {
		changed := false
		for uri, edits := range chWrap.Changes {
			if !isVaneURI(uri) {
				continue
			}
			var keptEdits []textEdit
			for _, edit := range edits {
				r, ok, _ := translateGoRangeToVaneForEdit(uri, edit.Range, store)
				if !ok {
					changed = true
					continue
				}
				edit.Range = r
				changed = true
				keptEdits = append(keptEdits, edit)
			}
			chWrap.Changes[uri] = keptEdits
		}
		if changed {
			b, _ := json.Marshal(chWrap)
			return json.RawMessage(b), true
		}
	}
	return editJSON, false
}

// translateApplyEdit intercepts workspace/applyEdit requests sent by gopls to
// the editor. These carry a WorkspaceEdit in params.edit whose ranges are still
// in go-coordinates. URIs have already been remapped by virtualToVane.
func translateApplyEdit(msg Message, store *docStore) Message {
	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Method  string          `json:"method"`
		Params  struct {
			Label string          `json:"label,omitempty"`
			Edit  json.RawMessage `json:"edit"`
		} `json:"params"`
	}
	if json.Unmarshal(msg, &req) != nil || req.Method != "workspace/applyEdit" || len(req.Params.Edit) == 0 {
		return msg
	}
	translated, changed := translateWorkspaceEditJSON(req.Params.Edit, store)
	if !changed {
		return msg
	}
	req.Params.Edit = translated
	b, err := json.Marshal(req)
	if err != nil {
		return msg
	}
	return Message(b)
}

// translateResponsePos translates position fields in responses back from compiled
// Go line coordinates to .vane line coordinates. Handles Location, []Location,
// and []LocationLink results (gopls returns LocationLink when the client
// advertises textDocument.definition.linkSupport, which VS Code does).
// Must be called AFTER virtualToVane so .vane URIs are present for store lookup.
func translateResponsePos(msg Message, store *docStore) Message {
	type location struct {
		URI   string   `json:"uri"`
		Range lspRange `json:"range"`
	}
	type locationLink struct {
		OriginSelectionRange *lspRange `json:"originSelectionRange,omitempty"`
		TargetURI            string    `json:"targetUri"`
		TargetRange          lspRange  `json:"targetRange"`
		TargetSelectionRange lspRange  `json:"targetSelectionRange"`
	}

	var resp struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Result  json.RawMessage `json:"result,omitempty"`
	}
	if json.Unmarshal(msg, &resp) != nil || len(resp.Result) == 0 || string(resp.Result) == "null" {
		return msg
	}

	translateRange := func(uri string, r lspRange) (lspRange, bool, bool) {
		return translateGoRangeToVane(uri, r, store)
	}

	nullResp := func() Message {
		resp.Result = json.RawMessage("null")
		r, _ := json.Marshal(resp)
		return Message(r)
	}

	// Try []LocationLink (gopls with linkSupport=true)
	var links []locationLink
	if json.Unmarshal(resp.Result, &links) == nil && len(links) > 0 && links[0].TargetURI != "" {
		changed := false
		var outLinks []locationLink
		for _, link := range links {
			if !isVaneURI(link.TargetURI) {
				outLinks = append(outLinks, link)
				continue
			}
			tr, trOK, trDrop := translateRange(link.TargetURI, link.TargetRange)
			tsr, tsrOK, tsrDrop := translateRange(link.TargetURI, link.TargetSelectionRange)
			if trDrop || tsrDrop {
				changed = true
				continue // drop preamble result
			}
			if trOK {
				link.TargetRange = tr
				changed = true
			}
			if tsrOK {
				link.TargetSelectionRange = tsr
				changed = true
			}
			outLinks = append(outLinks, link)
		}
		if changed {
			if len(outLinks) == 0 {
				return nullResp()
			}
			newResult, _ := json.Marshal(outLinks)
			resp.Result = json.RawMessage(newResult)
			result, _ := json.Marshal(resp)
			return Message(result)
		}
	}

	// Try []Location
	var locs []location
	if json.Unmarshal(resp.Result, &locs) == nil && len(locs) > 0 && locs[0].URI != "" {
		changed := false
		var outLocs []location
		for _, loc := range locs {
			if !isVaneURI(loc.URI) {
				outLocs = append(outLocs, loc)
				continue
			}
			r, ok, drop := translateRange(loc.URI, loc.Range)
			if drop {
				changed = true
				continue // drop preamble result
			}
			if ok {
				loc.Range = r
				changed = true
			}
			outLocs = append(outLocs, loc)
		}
		if changed {
			if len(outLocs) == 0 {
				return nullResp()
			}
			newResult, _ := json.Marshal(outLocs)
			resp.Result = json.RawMessage(newResult)
			result, _ := json.Marshal(resp)
			return Message(result)
		}
	}

	// Try single Location
	var loc location
	if json.Unmarshal(resp.Result, &loc) == nil && loc.URI != "" && isVaneURI(loc.URI) {
		r, ok, drop := translateRange(loc.URI, loc.Range)
		if drop {
			return nullResp()
		}
		if ok {
			loc.Range = r
			newResult, _ := json.Marshal(loc)
			resp.Result = json.RawMessage(newResult)
			result, _ := json.Marshal(resp)
			return Message(result)
		}
	}

	// Try WorkspaceEdit (result of textDocument/rename).
	if translated, changed := translateWorkspaceEditJSON(resp.Result, store); changed {
		resp.Result = translated
		result, _ := json.Marshal(resp)
		return Message(result)
	}

	return msg
}

// startGopls starts the gopls process with GOOS=js and GOARCH=wasm in its environment.
func startGopls() (*exec.Cmd, error) {
	path, err := exec.LookPath("gopls")
	if err != nil {
		return nil, fmt.Errorf("gopls not found in PATH: %w", err)
	}
	cmd := exec.Command(path, "serve") // #nosec G204 -- path is resolved via exec.LookPath("gopls") from the local PATH, not attacker input
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	return cmd, nil
}

// handleWatchedFilesChange processes workspace/didChangeWatchedFiles:
//   - .vane created/renamed-to: compile and write _vane.go
//   - .vane deleted/renamed-from: remove the corresponding _vane.go
//   - _vane.go deleted: regenerate from .vane source on disk
//
// File change type values: 1=Created, 2=Changed, 3=Deleted.
func handleWatchedFilesChange(msg Message, goplsIn io.Writer, store *docStore) {
	var notif struct {
		Params struct {
			Changes []struct {
				URI  string `json:"uri"`
				Type int    `json:"type"`
			} `json:"changes"`
		} `json:"params"`
	}
	if json.Unmarshal(msg, &notif) != nil {
		return
	}

	compileAndWrite := func(uri string) {
		vanePath := uriToPath(uri)
		src, err := os.ReadFile(vanePath) // #nosec G304 -- vanePath comes from the local editor's own LSP messages over stdio, same trust boundary as the user running it
		if err != nil {
			fmt.Fprintf(os.Stderr, "[vane lsp] watchedFiles: read error for %s: %v\n", vanePath, err)
			return
		}
		goContent, sm, err := store.compile(uri, string(src))
		if err != nil {
			fmt.Fprintf(os.Stderr, "[vane lsp] watchedFiles: compile error for %s: %v\n", vanePath, err)
			return
		}
		store.set(uri, string(src), goContent, sm)
		writeToDisk(uri, goContent)
	}

	for _, ch := range notif.Params.Changes {
		uri := ch.URI
		switch {
		case isVaneURI(uri) && ch.Type == 3: // .vane deleted/renamed-from
			genPath := strings.TrimSuffix(uriToPath(uri), ".vane") + "_vane.go"
			_ = os.Remove(genPath)
			store.delete(uri)
		case isVaneURI(uri) && (ch.Type == 1 || ch.Type == 2): // .vane created/renamed-to
			compileAndWrite(uri)
		case strings.HasSuffix(uri, "_vane.go") && ch.Type == 3: // _vane.go deleted
			vanePath := strings.TrimSuffix(uriToPath(uri), "_vane.go") + ".vane"
			vaneURI := pathToFileURI(vanePath)
			compileAndWrite(vaneURI)
		}
	}
}

// dropRedundantVaneGoChanges strips Created/Changed entries for _vane.go files
// out of a workspace/didChangeWatchedFiles notification before it reaches gopls.
// Every such entry is VS Code's file watcher echoing back a write we ourselves
// just made in handleDidChange/handleDidOpen, which already told gopls about
// the new content synchronously via a virtual-document didChange/didOpen. That
// echo is pure noise on top of the authoritative overlay update, and forwarding
// it made gopls invalidate its snapshot concurrently with in-flight completion
// and codeAction requests, confirmed live as the cause of gopls's "column is
// beyond end of line" failures, even for positions proven byte-for-byte correct.
// Deleted (_vane.go) and any .vane entries are left untouched and still forwarded.
// Returns (filteredMsg, false) when nothing is left to forward.
func dropRedundantVaneGoChanges(msg Message) (Message, bool) {
	var notif struct {
		Params struct {
			Changes []struct {
				URI  string `json:"uri"`
				Type int    `json:"type"`
			} `json:"changes"`
		} `json:"params"`
	}
	if json.Unmarshal(msg, &notif) != nil {
		return msg, true
	}

	kept := notif.Params.Changes[:0]
	for _, ch := range notif.Params.Changes {
		if strings.HasSuffix(ch.URI, "_vane.go") && (ch.Type == 1 || ch.Type == 2) {
			continue
		}
		kept = append(kept, ch)
	}
	if len(kept) == 0 {
		return msg, false
	}
	if len(kept) == len(notif.Params.Changes) {
		return msg, true // nothing dropped, forward unchanged
	}

	var generic map[string]json.RawMessage
	if json.Unmarshal(msg, &generic) != nil {
		return msg, true
	}
	var params map[string]json.RawMessage
	if json.Unmarshal(generic["params"], &params) != nil {
		return msg, true
	}
	newChanges, _ := json.Marshal(kept)
	params["changes"] = json.RawMessage(newChanges)
	newParams, _ := json.Marshal(params)
	generic["params"] = json.RawMessage(newParams)
	result, err := json.Marshal(generic)
	if err != nil {
		return msg, true
	}
	return Message(result), true
}

// precompileWorkspace compiles every .vane file under root and writes the
// resulting _vane.go files to disk alongside the source. This must happen
// before gopls starts its packages.Load so go list finds the generated files
// and includes them in package metadata. Overlays sent after packages.Load
// are NOT reflected in go list results and therefore don't contribute symbols
// to type-checking, only disk files do.
func precompileWorkspace(root string, store *docStore) {
	count := 0
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".vane") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			if part == "vendor" {
				return nil
			}
		}
		content, err := os.ReadFile(path) // #nosec G304 G122 -- path is discovered by walking the editor's own workspace root, not attacker input
		if err != nil {
			return nil
		}
		uri := pathToFileURI(path)
		goContent, sm, err := store.compile(uri, string(content))
		if err != nil {
			return nil
		}
		store.set(uri, string(content), goContent, sm)
		writeToDisk(uri, goContent)
		count++
		return nil
	})
	fmt.Fprintf(os.Stderr, "[vane lsp] precompile: wrote %d _vane.go files to disk\n", count)
}

// stripLineDirectives removes //line directives from Go source before it is
// written to disk or sent to gopls. The source map is already built from them;
// leaving them causes gopls to follow //line to .vane files (not valid Go) and
// refuse operations like rename/prepareRename.
func stripLineDirectives(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "//line ") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// writeToDisk writes compiled Go content to the _vane.go file on disk.
// The file lives next to its source .vane file so go list and gopls can
// discover it as part of the package. Files are listed in .gitignore.
// //line directives are stripped so gopls does not follow them to .vane files.
func writeToDisk(vaneURI, goContent string) {
	vanePath := uriToPath(vaneURI)
	goPath := strings.TrimSuffix(vanePath, ".vane") + "_vane.go"
	if err := os.WriteFile(goPath, []byte(stripLineDirectives(goContent)), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "[vane lsp] writeToDisk error for %s: %v\n", goPath, err)
	}
}
