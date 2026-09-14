package lsp

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Message is a raw LSP/JSONRPC message body (JSON bytes).
type Message []byte

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
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(msg))
	buf := make([]byte, 0, len(header)+len(msg))
	buf = append(buf, header...)
	buf = append(buf, msg...)
	_, err := w.Write(buf)
	return err
}
