package lsp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Message is a raw LSP/JSONRPC message body (JSON bytes).
type Message []byte

// maxMessageSize bounds LSP payloads read or written - 64 MiB is comfortably
// below int overflow range while far above any real LSP message, so a
// malformed or malicious Content-Length can't drive an unbounded allocation.
const maxMessageSize = 64 * 1024 * 1024

// ReadMessage reads one LSP message from r.
// Format: "Content-Length: N\r\n\r\n" followed by N bytes of JSON.
func ReadMessage(r *bufio.Reader) (Message, error) {
	contentLength := -1

	// Read headers until blank line.
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			n, err := strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
			contentLength = n
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	if contentLength > maxMessageSize {
		return nil, fmt.Errorf("Content-Length too large: %d", contentLength)
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	return Message(body), nil
}

// WriteMessage writes one LSP message to w, as a single Write call - not
// header then body separately - so a caller can wrap w with a mutex and get
// true per-message atomicity even with multiple concurrent writers (see
// internal/lsp's keyed-for resolution, which writes to gopls's stdin from
// its own goroutine alongside the main editor-proxy loop).
func WriteMessage(w io.Writer, msg Message) error {
	msgLen := len(msg)
	if msgLen > maxMessageSize {
		return fmt.Errorf("message too large: %d", msgLen)
	}
	// bytes.Buffer grows its own backing array internally rather than this
	// function pre-computing a capacity from two lengths added together -
	// still exactly one w.Write call at the end, preserving the atomicity
	// this function exists for.
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Content-Length: %d\r\n\r\n", msgLen)
	buf.Write(msg)
	_, err := w.Write(buf.Bytes())
	return err
}
