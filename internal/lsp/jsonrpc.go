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

// WriteMessage writes one LSP message to w.
func WriteMessage(w io.Writer, msg Message) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(msg))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err := w.Write(msg)
	return err
}
