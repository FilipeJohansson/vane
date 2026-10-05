package lsp

import "strings"

// repairEditedLine makes unfinished code compilable so the editor keeps
// getting completion and hover while it is being typed. text is the document
// as it now stands and prev what the server last saw: on the first line that
// differs, any bracket left open ((, [ or {) is closed at the end of that
// line. Only appending to a line keeps every position before the cursor
// valid. It reports false when there is no previous version, no differing
// line, or nothing to close.
func repairEditedLine(prev, text string) (string, bool) {
	oldLines := strings.Split(prev, "\n")
	lines := strings.Split(text, "\n")
	i := 0
	for i < len(lines) && i < len(oldLines) && lines[i] == oldLines[i] {
		i++
	}
	if i >= len(lines) {
		return "", false
	}
	closers := missingClosers(lines[i])
	if closers == "" {
		return "", false
	}
	// A line ending in "\r" keeps it after the appended closers.
	lines[i] = strings.TrimSuffix(lines[i], "\r") + closers
	if strings.Contains(text, "\r\n") {
		lines[i] += "\r"
	}
	return strings.Join(lines, "\n"), true
}

// missingClosers returns the closing brackets, innermost first, needed to
// balance the brackets opened on line. Brackets inside strings, runes and
// trailing // comments are ignored. A closer that does not match the open
// bracket makes the line unrepairable (empty result).
func missingClosers(line string) string {
	var stack []byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch c {
		case '"':
			for i++; i < len(line) && line[i] != '"'; i++ {
				if line[i] == '\\' {
					i++
				}
			}
		case '`':
			end := strings.IndexByte(line[i+1:], '`')
			if end < 0 {
				return ""
			}
			i += 1 + end
		case '\'':
			// A rune literal is at most a few bytes; a lone apostrophe (e.g. in
			// JSX text) is just a character.
			if end := strings.IndexByte(line[i+1:], '\''); end >= 0 && end <= 5 {
				i += 1 + end
			}
		case '/':
			if i+1 < len(line) && line[i+1] == '/' {
				i = len(line)
			}
		case '(', '[', '{':
			stack = append(stack, c)
		case ')', ']', '}':
			if len(stack) == 0 {
				continue
			}
			want := map[byte]byte{')': '(', ']': '[', '}': '{'}[c]
			if stack[len(stack)-1] != want {
				return ""
			}
			stack = stack[:len(stack)-1]
		}
	}
	var b strings.Builder
	for j := len(stack) - 1; j >= 0; j-- {
		b.WriteByte(map[byte]byte{'(': ')', '[': ']', '{': '}'}[stack[j]])
	}
	return b.String()
}
