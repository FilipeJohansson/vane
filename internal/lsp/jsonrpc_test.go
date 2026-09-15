package lsp

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// TestReadWriteMessage_RoundTrips confirms a normal message survives a
// WriteMessage -> ReadMessage round trip unchanged.
func TestReadWriteMessage_RoundTrips(t *testing.T) {
	var buf bytes.Buffer
	want := Message(`{"jsonrpc":"2.0","method":"test"}`)
	if err := WriteMessage(&buf, want); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	got, err := ReadMessage(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestReadMessage_RejectsOversizedContentLength is a regression test for a
// real CodeQL finding: Content-Length came straight from an untrusted header
// with no upper bound, so a malformed or malicious peer claiming an
// enormous size could drive an unbounded allocation before ReadMessage ever
// tried to read a single body byte. maxMessageSize caps it - this asserts
// the rejection happens for a size well past any real LSP message, without
// this test itself needing to construct anywhere near that many bytes.
func TestReadMessage_RejectsOversizedContentLength(t *testing.T) {
	header := "Content-Length: 999999999999\r\n\r\n"
	r := bufio.NewReader(strings.NewReader(header))
	_, err := ReadMessage(r)
	if err == nil {
		t.Fatal("expected an error for a Content-Length far exceeding maxMessageSize, got none")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("error doesn't explain the real cause: %v", err)
	}
}

// TestReadMessage_MissingContentLength confirms the existing, unrelated
// error path (no Content-Length header at all) still works after adding
// the size-bound check right next to it.
func TestReadMessage_MissingContentLength(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("\r\n"))
	_, err := ReadMessage(r)
	if err == nil {
		t.Fatal("expected an error for a missing Content-Length header, got none")
	}
	if !strings.Contains(err.Error(), "missing Content-Length") {
		t.Fatalf("error doesn't explain the real cause: %v", err)
	}
}

// TestWriteMessage_RejectsOversizedMessage is WriteMessage's own side of the
// same size-bound fix.
func TestWriteMessage_RejectsOversizedMessage(t *testing.T) {
	huge := make(Message, maxMessageSize+1)
	var buf bytes.Buffer
	err := WriteMessage(&buf, huge)
	if err == nil {
		t.Fatal("expected an error for a message exceeding maxMessageSize, got none")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("error doesn't explain the real cause: %v", err)
	}
}
