package lsp

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const staleSrcTemplate = `package main

import "github.com/filipejohansson/vane/core"

func App() core.Node {
	return <div>%s</div>
}
`

func staleSrc(label string) string {
	return strings.Replace(staleSrcTemplate, "%s", label, 1)
}

// An editor buffer holds an edit that has not been saved yet, so the file on
// disk is older than the in-memory document. A workspace/didChangeWatchedFiles
// event for that file (the watcher reporting an earlier save, delivered late)
// must not replace the newer in-memory document with the older disk content.
func TestWatchedFilesChange_DoesNotClobberNewerInMemoryEdit(t *testing.T) {
	dir := t.TempDir()
	vanePath := filepath.Join(dir, "Stale.vane")
	diskSrc := staleSrc("saved")
	bufferSrc := staleSrc("typed after save")

	if err := os.WriteFile(vanePath, []byte(diskSrc), 0o600); err != nil {
		t.Fatal(err)
	}
	uri := pathToFileURI(vanePath)
	store := newDocStore()

	open, _ := json.Marshal(map[string]any{
		"method": "textDocument/didOpen",
		"params": map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "vane", "version": 1, "text": diskSrc}},
	})
	if !handleDidOpen(Message(open), io.Discard, store) {
		t.Fatal("didOpen not handled")
	}
	change, _ := json.Marshal(map[string]any{
		"method": "textDocument/didChange",
		"params": map[string]any{
			"textDocument":   map[string]any{"uri": uri, "version": 2},
			"contentChanges": []map[string]any{{"text": bufferSrc}},
		},
	})
	if !handleDidChange(Message(change), io.Discard, store) {
		t.Fatal("didChange not handled")
	}

	watched, _ := json.Marshal(map[string]any{
		"method": "workspace/didChangeWatchedFiles",
		"params": map[string]any{"changes": []map[string]any{{"uri": uri, "type": 2}}},
	})
	handleWatchedFilesChange(Message(watched), io.Discard, store)

	doc, ok := store.get(uri)
	if !ok {
		t.Fatal("document missing from store")
	}
	got := strings.Join(doc.vaneLines, "\n")
	if got != bufferSrc {
		t.Errorf("in-memory edit was overwritten by stale disk content:\n got: %q\nwant: %q", got, bufferSrc)
	}
}

// A deleted _vane.go for an open document is regenerated from the editor
// buffer, including unsaved edits, not from the older file on disk.
func TestWatchedFilesChange_RegeneratesDeletedGoFileFromBuffer(t *testing.T) {
	dir := t.TempDir()
	vanePath := filepath.Join(dir, "Stale.vane")
	goPath := filepath.Join(dir, "Stale_vane.go")
	diskSrc := staleSrc("saved")
	bufferSrc := staleSrc("typed after save")

	if err := os.WriteFile(vanePath, []byte(diskSrc), 0o600); err != nil {
		t.Fatal(err)
	}
	uri := pathToFileURI(vanePath)
	store := newDocStore()

	open, _ := json.Marshal(map[string]any{
		"method": "textDocument/didOpen",
		"params": map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "vane", "version": 1, "text": diskSrc}},
	})
	handleDidOpen(Message(open), io.Discard, store)
	change, _ := json.Marshal(map[string]any{
		"method": "textDocument/didChange",
		"params": map[string]any{
			"textDocument":   map[string]any{"uri": uri, "version": 2},
			"contentChanges": []map[string]any{{"text": bufferSrc}},
		},
	})
	handleDidChange(Message(change), io.Discard, store)

	if err := os.Remove(goPath); err != nil {
		t.Fatalf("generated file should exist after didOpen: %v", err)
	}
	watched, _ := json.Marshal(map[string]any{
		"method": "workspace/didChangeWatchedFiles",
		"params": map[string]any{"changes": []map[string]any{{"uri": pathToFileURI(goPath), "type": 3}}},
	})
	handleWatchedFilesChange(Message(watched), io.Discard, store)

	regenerated, err := os.ReadFile(goPath)
	if err != nil {
		t.Fatalf("_vane.go was not regenerated: %v", err)
	}
	if !strings.Contains(string(regenerated), "typed after save") {
		t.Errorf("regenerated file is missing the unsaved edit:\n%s", regenerated)
	}
	if strings.Contains(string(regenerated), ">saved<") {
		t.Errorf("regenerated file was built from stale disk content:\n%s", regenerated)
	}
}

// The language server can be restarted while a file is already open, so a
// document may be edited (didChange) without this server ever seeing its
// didOpen; its precompiled entry is already in the store. The buffer still
// wins over the disk content.
func TestWatchedFilesChange_EditWithoutDidOpenStillWinsOverDisk(t *testing.T) {
	dir := t.TempDir()
	vanePath := filepath.Join(dir, "Stale.vane")
	diskSrc := staleSrc("saved")
	bufferSrc := staleSrc("typed after save")

	if err := os.WriteFile(vanePath, []byte(diskSrc), 0o600); err != nil {
		t.Fatal(err)
	}
	uri := pathToFileURI(vanePath)
	store := newDocStore()
	goContent, sm, err := store.compile(uri, diskSrc)
	if err != nil {
		t.Fatal(err)
	}
	store.set(uri, diskSrc, goContent, sm) // as precompileWorkspace does

	change, _ := json.Marshal(map[string]any{
		"method": "textDocument/didChange",
		"params": map[string]any{
			"textDocument":   map[string]any{"uri": uri, "version": 2},
			"contentChanges": []map[string]any{{"text": bufferSrc}},
		},
	})
	handleDidChange(Message(change), io.Discard, store)

	watched, _ := json.Marshal(map[string]any{
		"method": "workspace/didChangeWatchedFiles",
		"params": map[string]any{"changes": []map[string]any{{"uri": uri, "type": 2}}},
	})
	handleWatchedFilesChange(Message(watched), io.Discard, store)

	if got, _ := store.text(uri); got != bufferSrc {
		t.Errorf("in-memory edit was overwritten by stale disk content:\n got: %q\nwant: %q", got, bufferSrc)
	}
}

// Closing a document without saving discards its edits: the store entry and
// the generated _vane.go go back to the saved source, and the document stays
// available for navigation into closed files.
func TestDidClose_DiscardedEditsRevertToSavedSource(t *testing.T) {
	dir := t.TempDir()
	vanePath := filepath.Join(dir, "Stale.vane")
	goPath := filepath.Join(dir, "Stale_vane.go")
	diskSrc := staleSrc("saved")
	bufferSrc := staleSrc("typed after save")

	if err := os.WriteFile(vanePath, []byte(diskSrc), 0o600); err != nil {
		t.Fatal(err)
	}
	uri := pathToFileURI(vanePath)
	store := newDocStore()

	open, _ := json.Marshal(map[string]any{
		"method": "textDocument/didOpen",
		"params": map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "vane", "version": 1, "text": diskSrc}},
	})
	handleDidOpen(Message(open), io.Discard, store)
	change, _ := json.Marshal(map[string]any{
		"method": "textDocument/didChange",
		"params": map[string]any{
			"textDocument":   map[string]any{"uri": uri, "version": 2},
			"contentChanges": []map[string]any{{"text": bufferSrc}},
		},
	})
	handleDidChange(Message(change), io.Discard, store)

	closeMsg, _ := json.Marshal(map[string]any{
		"method": "textDocument/didClose",
		"params": map[string]any{"textDocument": map[string]any{"uri": uri}},
	})
	if !handleDidClose(Message(closeMsg), io.Discard, store) {
		t.Fatal("didClose not handled")
	}

	if store.isOpen(uri) {
		t.Error("document still marked open after didClose")
	}
	got, ok := store.text(uri)
	if !ok {
		t.Fatal("closed document was removed from the store; navigation into closed files needs it")
	}
	if got != diskSrc {
		t.Errorf("store still holds the discarded edit:\n got: %q\nwant: %q", got, diskSrc)
	}
	generated, err := os.ReadFile(goPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(generated), "typed after save") {
		t.Errorf("generated file still contains the discarded edit:\n%s", generated)
	}
}
