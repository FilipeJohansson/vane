package compiler

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/filipejohansson/vane/core/domattrs"
)

// PosEntry is a single entry in a SourceMap: a .vane line mapped to a generated Go line.
// Both fields are 0-based.
type PosEntry struct {
	VaneLine int
	GoLine   int
}

// SourceMap provides bidirectional line-level mapping between .vane source and
// the compiled Go output. Built from //line directives in the generated code.
type SourceMap struct {
	byVane []PosEntry // sorted by VaneLine
	byGo   []PosEntry // sorted by GoLine (monotonically increasing in generated output)
}

// VaneToGo maps a 0-based .vane line to the nearest Go line in compiled output.
// col is passed through unchanged (column-level mapping not yet implemented).
func (m *SourceMap) VaneToGo(line, col int) (goLine, goCol int, ok bool) {
	if len(m.byVane) == 0 {
		return 0, 0, false
	}
	entries := m.byVane
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := (lo + hi) / 2
		if entries[mid].VaneLine <= line {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo == 0 {
		return 0, 0, false
	}
	e := entries[lo-1]
	return e.GoLine + (line - e.VaneLine), col, true
}

// GoLinesForVaneLine returns all Go line numbers that are directly mapped from
// the given 0-based vane line (i.e. have an explicit //line directive for it).
// Used by the LSP proxy to search sibling go lines when mapColumn fails on the
// primary mapped line (e.g. multiple attributes on the same vane line).
func (m *SourceMap) GoLinesForVaneLine(vaneLine int) []int {
	entries := m.byVane
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := (lo + hi) / 2
		if entries[mid].VaneLine < vaneLine {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	var result []int
	for lo < len(entries) && entries[lo].VaneLine == vaneLine {
		result = append(result, entries[lo].GoLine)
		lo++
	}
	return result
}

// GoToVane maps a 0-based Go line in compiled output back to the .vane source line.
// col is passed through unchanged.
func (m *SourceMap) GoToVane(line, col int) (vaneLine, vaneCol int, ok bool) {
	if len(m.byGo) == 0 {
		return 0, 0, false
	}
	entries := m.byGo
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := (lo + hi) / 2
		if entries[mid].GoLine <= line {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo == 0 {
		return 0, 0, false
	}
	e := entries[lo-1]
	return e.VaneLine + (line - e.GoLine), col, true
}

// buildSourceMap parses //line directives in generated Go source and returns a SourceMap.
// GoLine values in the returned entries are 0-based line indices in the STRIPPED file
// (i.e. the file as gopls sees it, with //line directives removed). This keeps GoLine
// values consistent with the content actually sent to gopls.
func buildSourceMap(goSrc string) *SourceMap {
	lines := strings.Split(goSrc, "\n")
	var byGo []PosEntry
	strippedLine := 0 // counts non-directive lines (= line index in stripped file)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//line ") {
			rest := trimmed[len("//line "):]
			colonIdx := strings.LastIndex(rest, ":")
			if colonIdx >= 0 {
				n, err := strconv.Atoi(rest[colonIdx+1:])
				if err == nil && n >= 1 {
					// directive removed from stripped file; next non-directive line is at strippedLine
					byGo = append(byGo, PosEntry{VaneLine: n - 1, GoLine: strippedLine})
				}
			}
			continue // //line line is removed; don't increment strippedLine
		}
		strippedLine++
	}
	byVane := make([]PosEntry, len(byGo))
	copy(byVane, byGo)
	sort.Slice(byVane, func(i, j int) bool {
		if byVane[i].VaneLine != byVane[j].VaneLine {
			return byVane[i].VaneLine < byVane[j].VaneLine
		}
		return byVane[i].GoLine < byVane[j].GoLine
	})
	return &SourceMap{byVane: byVane, byGo: byGo}
}

// CompileWithMap transforms .vane source to .go source and returns a SourceMap for LSP position translation.
func CompileWithMap(src, filename string) (string, *SourceMap, error) {
	s := &scanner{src: src, filename: filename}
	out, err := s.scan()
	if err != nil {
		return "", nil, err
	}
	// Only inject syscall/js when the generated code actually references it.
	// Most vane files now return core.Node and never touch js.* directly, so forcing
	// the import unconditionally would produce an "imported and not used" error.
	// Comments and strings are stripped first so a mention of "js." in prose (e.g. a
	// doc comment) doesn't cause a false positive.
	if strings.Contains(stripCommentsAndStrings(out), "js.") {
		out = injectImport(out, `"syscall/js"`)
	}
	lineAnchor := ""
	if filename != "" {
		lineAnchor = "//line " + filename + ":1\n"
	}
	header := "//go:build js && wasm\n\n"
	if strings.Contains(src, "//go:build") {
		header = ""
	}
	goSrc := header + lineAnchor + out
	return goSrc, buildSourceMap(goSrc), nil
}

// Compile transforms .vane source (Go + vane syntax) to valid .go source.
// vane syntax inside return statements is converted to fine-grained DOM calls.
// filename is used only for //line directives in the generated output so that
// Go compiler errors point back to the original .vane file; pass "" to disable.
func Compile(src, filename string) (string, error) {
	out, _, err := CompileWithMap(src, filename)
	return out, err
}

// stripCommentsAndStrings removes // and /* */ comments plus string/rune/raw
// literals from src, replacing each with a single space. Used to search generated
// code for real symbol usage without false-positiving on mentions inside comments or strings.
func stripCommentsAndStrings(src string) string {
	s := &scanner{src: src}
	var out strings.Builder
	for !s.atEnd() {
		c := s.cur()
		switch {
		case c == '"' || c == '\'' || c == '`':
			s.readString()
			out.WriteByte(' ')
		case c == '/' && s.peek(1) == '/':
			s.readLineComment()
			out.WriteByte(' ')
		case c == '/' && s.peek(1) == '*':
			s.readBlockComment()
			out.WriteByte(' ')
		default:
			out.WriteByte(c)
			s.pos++
		}
	}
	return out.String()
}

func injectImport(src, imp string) string {
	if strings.Contains(src, imp) {
		return src
	}
	if idx := strings.Index(src, "import ("); idx != -1 {
		insertAt := idx + len("import (")
		return src[:insertAt] + "\n\t" + imp + src[insertAt:]
	}
	// No import block yet, or a single-line one: inject right after package.
	if idx := strings.Index(src, "package "); idx != -1 {
		end := strings.Index(src[idx:], "\n")
		if end != -1 {
			insertAt := idx + end + 1
			return src[:insertAt] + "\nimport " + imp + "\n" + src[insertAt:]
		}
	}
	return src
}

//* AST

// elementSugar returns the "return nil" replacement expression for a func
// signature/body fragment, based on its declared element return type:
// "core.Empty()" for core.Node, "js.Undefined()" for the legacy js.Value.
// Returns "" if the fragment declares neither (return nil should not be rewritten there).
func elementSugar(s string) string {
	if strings.Contains(s, "core.Node") {
		return "core.Empty()"
	}
	if strings.Contains(s, "js.Value") {
		return "js.Undefined()"
	}
	return ""
}

// returnsElement reports whether a func signature/body fragment declares an
// element-returning type (either the legacy "js.Value" or "core.Node").
func returnsElement(s string) bool {
	return elementSugar(s) != ""
}

type node interface{ isNode() }

type elemNode struct {
	tag      string
	pos      int // byte offset of '<' in source
	attrs    []vaneAttr
	children []node
}

type textNode struct {
	content string
	pos     int
}

type exprNode struct {
	code   string
	spread bool
	pos    int // byte offset of '{' in source
}

func (*elemNode) isNode() {}
func (*textNode) isNode() {}
func (*exprNode) isNode() {}

// ctrlFlowNode represents {for/if/switch ...} in vane syntax child position.
type ctrlFlowNode struct {
	raw string
	pos int // byte offset of '{' in source
}

func (*ctrlFlowNode) isNode() {}

func isControlFlow(s string) bool {
	return strings.HasPrefix(s, "for ") || strings.HasPrefix(s, "if ") ||
		strings.HasPrefix(s, "switch ") || strings.HasPrefix(s, "switch{")
}

// bodyPart is a segment of a control-flow block body.
// Exactly one of vane (a parsed vane element) or callExpr is set, or neither for pure Go setup code.
type bodyPart struct {
	goCode   string
	vane     *elemNode
	callExpr string // Go expression returning js.Value (e.g. component call)
}

type switchCaseData struct {
	caseExpr  string
	body      string
	bodyStart int // byte offset of body within the switch body string (for //line directives)
}

type vaneAttr struct {
	name     string
	value    string
	isExpr   bool
	namePos  int // byte offset of attribute name in source
	valuePos int // byte offset of attribute value ('{' or '"') in source
}

//* Scanner

type scanner struct {
	src      string
	pos      int
	filename string
}

// lineAt counts the 1-based line number of byte offset pos in src.
func lineAt(src string, pos int) int {
	line := 1
	for i := 0; i < pos && i < len(src); i++ {
		if src[i] == '\n' {
			line++
		}
	}
	return line
}

//* Rich error formatting

// ParseError is a compiler error with source location and context.
type ParseError struct {
	filename string
	src      string
	pos      int // byte offset in src
	msg      string
	hint     string
}

func (e *ParseError) Error() string {
	return formatParseError(e.filename, e.src, e.pos, e.msg, e.hint)
}

// Format returns a styled string for CLI display. useColor enables ANSI color codes.
// Error() is preserved unchanged for tests and error wrapping.
func (e *ParseError) Format(useColor bool) string {
	reset, bold, dim, red, yellow := "", "", "", "", ""
	if useColor {
		reset = "\033[0m"
		bold = "\033[1m"
		dim = "\033[2m"
		red = "\033[31m"
		yellow = "\033[33m"
	}

	line, col := lineColAt(e.src, e.pos)

	var b strings.Builder

	// Header: filename:line:col · message
	filename := e.filename
	if filename == "" {
		filename = "<input>"
	}
	loc := fmt.Sprintf("%d:%d", line, col)
	fmt.Fprintf(&b, "  %s%s:%s%s %s·%s %s%s%s\n",
		bold, filename, loc, reset,
		dim, reset,
		red+bold, e.msg, reset)

	// Source context: 2 lines before, error line, 2 lines after
	srcLines := strings.Split(e.src, "\n")
	first := max(0, line-3)
	last := min(len(srcLines), line+2)
	numWidth := len(fmt.Sprintf("%d", last))

	for i := first; i < last; i++ {
		lineNum := i + 1
		isError := lineNum == line

		content := ""
		if i < len(srcLines) {
			content = strings.ReplaceAll(srcLines[i], "\t", "  ")
		}

		b.WriteString("  ")
		if isError {
			b.WriteString(bold)
		} else {
			b.WriteString(dim)
		}
		fmt.Fprintf(&b, "%*d", numWidth, lineNum)
		fmt.Fprintf(&b, "%s ", reset)

		if isError {
			fmt.Fprintf(&b, "%s→%s", red, reset)
		} else {
			fmt.Fprintf(&b, "%s %s", dim, reset)
		}
		fmt.Fprintf(&b, "%s│%s ", dim, reset)
		b.WriteString(content)
		b.WriteString("\n")

		// Caret under the error column (visual position accounting for tab expansion)
		if isError && col > 0 {
			rawLine := ""
			if i < len(srcLines) {
				rawLine = srcLines[i]
			}
			visualPad := tabExpandedWidth(rawLine, col-1)
			fmt.Fprintf(&b, "  %s  %s│%s %s%s^%s\n",
				strings.Repeat(" ", numWidth),
				dim, reset,
				red, strings.Repeat(" ", visualPad), reset)
		}
	}

	// Hint
	if e.hint != "" {
		fmt.Fprintf(&b, "\n  %s%sHint:%s %s\n", yellow, bold, reset, e.hint)
	}

	return b.String()
}

// tabExpandedWidth returns the visual column of bytePos in line, expanding
// each tab to 2 spaces.
func tabExpandedWidth(line string, bytePos int) int {
	visual := 0
	for i := 0; i < bytePos && i < len(line); i++ {
		if line[i] == '\t' {
			visual += 2
		} else {
			visual++
		}
	}
	return visual
}

// errorf creates a ParseError at the given byte offset.
func (s *scanner) errorf(pos int, msg, hint string) error {
	return &ParseError{filename: s.filename, src: s.src, pos: pos, msg: msg, hint: hint}
}

// lineColAt converts a byte offset to 1-based (line, col).
func lineColAt(src string, pos int) (line, col int) {
	line, col = 1, 1
	for i := 0; i < pos && i < len(src); i++ {
		if src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return
}

func formatParseError(filename, src string, pos int, msg, hint string) string {
	line, col := lineColAt(src, pos)

	var b strings.Builder

	// Header: file:line:col
	if filename != "" {
		fmt.Fprintf(&b, "%s:%d:%d\n", filename, line, col)
	} else {
		fmt.Fprintf(&b, "line %d:%d\n", line, col)
	}
	b.WriteByte('\n')

	// Error message
	fmt.Fprintf(&b, "  %s\n", msg)
	b.WriteByte('\n')

	// Source context: 2 lines before, error line (with caret), 2 lines after
	srcLines := strings.Split(src, "\n")
	first := max(0, line-3)
	last := min(len(srcLines), line+2)
	numWidth := len(fmt.Sprintf("%d", last))

	for i := first; i < last; i++ {
		lineNum := i + 1
		fmt.Fprintf(&b, "  %*d | %s\n", numWidth, lineNum, srcLines[i])
		if lineNum == line {
			pad := strings.Repeat(" ", col-1)
			fmt.Fprintf(&b, "  %*s | %s^\n", numWidth, "", pad)
		}
	}

	// Hint
	if hint != "" {
		b.WriteByte('\n')
		fmt.Fprintf(&b, "  Hint: %s\n", hint)
	}

	return b.String()
}

// wrapParseErr passes a *ParseError through unchanged; wraps other errors with context.
func wrapParseErr(context string, err error) error {
	var pe *ParseError
	if errors.As(err, &pe) {
		return err
	}
	return fmt.Errorf("%s: %w", context, err)
}

// looksLikeVane returns true if the current position is the start of a vane element
// (opening tag, self-closing tag, or fragment: <tag, <Tag/, <>).
// Does NOT advance s.pos.
func (s *scanner) looksLikeVane() bool {
	if s.atEnd() || s.cur() != '<' {
		return false
	}
	saved := s.pos
	s.pos++ // skip '<'

	// Fragment: <>
	if !s.atEnd() && s.cur() == '>' {
		s.pos = saved
		return true
	}

	if s.atEnd() || !isVaneTagStart(s.cur()) {
		s.pos = saved
		return false
	}

	// Read tag name
	for !s.atEnd() && (isVaneTagStart(s.cur()) || (s.cur() >= '0' && s.cur() <= '9') || s.cur() == '.') {
		s.pos++
	}
	s.skipWS()

	var result bool
	if !s.atEnd() {
		c := s.cur()
		// After tag name: '>' or '/>' or an attribute name → vane element
		// An operator or digit after tag name → comparison
		result = c == '>' || c == '/' || isVaneTagStart(c)
	}

	s.pos = saved
	return result
}

func (s *scanner) atEnd() bool { return s.pos >= len(s.src) }
func (s *scanner) cur() byte {
	if s.atEnd() {
		return 0
	}
	return s.src[s.pos]
}
func (s *scanner) peek(n int) byte {
	i := s.pos + n
	if i >= len(s.src) {
		return 0
	}
	return s.src[i]
}

func (s *scanner) skipWS() {
	for !s.atEnd() {
		c := s.cur()
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			s.pos++
		} else {
			break
		}
	}
}

func (s *scanner) readIdent() string {
	start := s.pos
	for !s.atEnd() {
		c := rune(s.src[s.pos])
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' {
			s.pos++
		} else {
			break
		}
	}
	return s.src[start:s.pos]
}

func (s *scanner) readAttrName() string {
	start := s.pos
	for !s.atEnd() {
		c := s.src[s.pos]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == ':' {
			s.pos++
		} else {
			break
		}
	}
	return s.src[start:s.pos]
}

func (s *scanner) isKeyword(kw string) bool {
	end := s.pos + len(kw)
	if end > len(s.src) || s.src[s.pos:end] != kw {
		return false
	}
	if s.pos > 0 {
		p := rune(s.src[s.pos-1])
		if unicode.IsLetter(p) || unicode.IsDigit(p) || p == '_' {
			return false
		}
	}
	if end < len(s.src) {
		n := rune(s.src[end])
		if unicode.IsLetter(n) || unicode.IsDigit(n) || n == '_' {
			return false
		}
	}
	return true
}

func (s *scanner) readString() string {
	start := s.pos
	q := s.src[s.pos]
	s.pos++
	if q == '`' {
		for !s.atEnd() {
			if s.src[s.pos] == '`' {
				s.pos++
				break
			}
			s.pos++
		}
	} else {
		for !s.atEnd() {
			c := s.src[s.pos]
			s.pos++
			if c == '\\' && !s.atEnd() {
				s.pos++
			} else if c == q {
				break
			}
		}
	}
	return s.src[start:s.pos]
}

func (s *scanner) lineNum(pos int) int {
	n := 1
	for i := 0; i < pos && i < len(s.src); i++ {
		if s.src[i] == '\n' {
			n++
		}
	}
	return n
}

func (s *scanner) readLineComment() string {
	start := s.pos
	for !s.atEnd() && s.cur() != '\n' {
		s.pos++
	}
	return s.src[start:s.pos]
}

func (s *scanner) readBlockComment() string {
	start := s.pos
	s.pos += 2
	for s.pos+1 < len(s.src) {
		if s.src[s.pos] == '*' && s.src[s.pos+1] == '/' {
			s.pos += 2
			break
		}
		s.pos++
	}
	return s.src[start:s.pos]
}

// readGoExpr reads a balanced {...} expression.
// s.pos must be just after the opening '{'.
// recompileExprFuncLiterals scans a Go expression string and, for every
// func literal whose signature contains "js.Value", recompiles its body so
// that vane return statements inside it are expanded to Go DOM calls.
// This enables inline IIFEs like {func() js.Value { return (<p>...</p>) }()} in vane syntax children.
func (s *scanner) recompileExprFuncLiterals(code string) (string, error) {
	sub := &scanner{src: code}
	var out strings.Builder

	for !sub.atEnd() {
		c := sub.cur()
		if c == '"' || c == '\'' || c == '`' {
			out.WriteString(sub.readString())
			continue
		}
		if c == '/' && sub.peek(1) == '/' {
			out.WriteString(sub.readLineComment())
			continue
		}
		if c == '/' && sub.peek(1) == '*' {
			out.WriteString(sub.readBlockComment())
			continue
		}

		if c == 'f' && sub.isKeyword("func") {
			funcKwStart := sub.pos
			sub.pos += 4 // skip "func"
			sigStart := sub.pos

			// Read signature until opening '{', tracking parens
			parenDepth := 0
			for !sub.atEnd() {
				cc := sub.cur()
				if cc == '"' || cc == '\'' || cc == '`' {
					sub.readString()
					continue
				}
				if cc == '/' && sub.peek(1) == '/' {
					sub.readLineComment()
					continue
				}
				if cc == '/' && sub.peek(1) == '*' {
					sub.readBlockComment()
					continue
				}
				if cc == '(' {
					parenDepth++
					sub.pos++
					continue
				}
				if cc == ')' {
					parenDepth--
					sub.pos++
					continue
				}
				if cc == '{' && parenDepth == 0 {
					break
				}
				sub.pos++
			}
			sig := code[sigStart:sub.pos]

			if !sub.atEnd() && sub.cur() == '{' && returnsElement(sig) {
				sub.pos++ // skip '{'
				bodyStart := sub.pos
				depth := 1
				for !sub.atEnd() && depth > 0 {
					cc := sub.cur()
					if cc == '"' || cc == '\'' || cc == '`' {
						sub.readString()
						continue
					}
					if cc == '/' && sub.peek(1) == '/' {
						sub.readLineComment()
						continue
					}
					if cc == '/' && sub.peek(1) == '*' {
						sub.readBlockComment()
						continue
					}
					if cc == '{' {
						depth++
					} else if cc == '}' {
						depth--
						if depth == 0 {
							break
						}
					}
					sub.pos++
				}
				body := code[bodyStart:sub.pos]
				if !sub.atEnd() {
					sub.pos++ // skip closing '}'
				}

				compiled, err := compileFuncBodyVane(body, s.filename, sig)
				if err != nil {
					return "", err
				}

				out.WriteString(code[funcKwStart:sigStart])
				out.WriteString(sig)
				out.WriteByte('{')
				out.WriteString(compiled)
				out.WriteByte('}')
				continue
			}

			// Not an element-returning func literal, so pass it through unchanged
			out.WriteString(code[funcKwStart:sub.pos])
			continue
		}

		out.WriteByte(c)
		sub.pos++
	}

	return out.String(), nil
}

// compileFuncBodyVane compiles the body of an element-returning function literal
// (return type core.Node or the legacy js.Value). It processes vane return
// statements and rewrites "return nil" to the appropriate empty-element sugar.
// Brace tracking ensures nested func literals that don't return an element type are not affected.
// outerSig is the enclosing func literal's signature, used to resolve the
// "return nil" sugar at the outermost brace depth.
func compileFuncBodyVane(body, filename, outerSig string) (string, error) {
	s := &scanner{src: body} // filename intentionally blank: body is a substring, line numbers would be wrong
	var out strings.Builder

	outerSugar := elementSugar(outerSig)

	var braceStack []bool
	var sugarStack []string
	nextBraceIsFunc := false

	currentSugar := func() string {
		for i := len(braceStack) - 1; i >= 0; i-- {
			if braceStack[i] {
				return sugarStack[i]
			}
		}
		return outerSugar // no nested func on stack = still inside the outer func
	}

	sugarBetweenLastFuncAndHere := func() string {
		end := s.pos
		funcIdx := strings.LastIndex(s.src[:end], "func")
		if funcIdx == -1 {
			return ""
		}
		return elementSugar(s.src[funcIdx:end])
	}

	for !s.atEnd() {
		c := s.cur()
		if c == '"' || c == '\'' || c == '`' {
			out.WriteString(s.readString())
			continue
		}
		if c == '/' && s.peek(1) == '/' {
			out.WriteString(s.readLineComment())
			continue
		}
		if c == '/' && s.peek(1) == '*' {
			out.WriteString(s.readBlockComment())
			continue
		}
		if c == '{' {
			sugar := ""
			if nextBraceIsFunc {
				sugar = sugarBetweenLastFuncAndHere()
			}
			braceStack = append(braceStack, nextBraceIsFunc)
			sugarStack = append(sugarStack, sugar)
			nextBraceIsFunc = false
			out.WriteByte(c)
			s.pos++
			continue
		}
		if c == '}' {
			if len(braceStack) > 0 {
				braceStack = braceStack[:len(braceStack)-1]
				sugarStack = sugarStack[:len(sugarStack)-1]
			}
			out.WriteByte(c)
			s.pos++
			continue
		}
		if c == 'f' && s.isKeyword("func") {
			nextBraceIsFunc = true
			out.WriteString("func")
			s.pos += 4
			continue
		}
		if c == 'r' && s.isKeyword("return") {
			result, _, err := s.handleReturn(currentSugar())
			if err != nil {
				return "", err
			}
			out.WriteString(result)
			continue
		}
		if c == '<' && s.looksLikeVane() {
			return "", fmt.Errorf("vane syntax element outside return statement: " +
				"vane syntax is only valid inside return ( ... ) blocks")
		}
		out.WriteByte(c)
		s.pos++
	}

	return out.String(), nil
}

func (s *scanner) readGoExpr() (string, bool, error) {
	depth := 1
	start := s.pos
	for !s.atEnd() {
		c := s.cur()
		if c == '"' || c == '\'' || c == '`' {
			s.readString()
			continue
		}
		if c == '/' && s.peek(1) == '/' {
			s.readLineComment()
			continue
		}
		if c == '/' && s.peek(1) == '*' {
			s.readBlockComment()
			continue
		}
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				content := strings.TrimSpace(s.src[start:s.pos])
				s.pos++
				if strings.HasSuffix(content, "...") {
					return content[:len(content)-3], true, nil
				}
				return content, false, nil
			}
		}
		s.pos++
	}
	return "", false, fmt.Errorf("unterminated Go expression")
}

//* Main scan loop

func (s *scanner) scan() (string, error) {
	var out strings.Builder

	// braceStack[i] == true  → the i-th open brace was a func body brace
	// braceStack[i] == false → if/for/switch/composite-literal brace
	// sugarStack[i] is valid only when braceStack[i]==true:
	//   "core.Empty()"/"js.Undefined()" → that func body's return type is an element type
	//   ""                              → other return type (func(), any, *T, etc.)
	var braceStack []bool
	var sugarStack []string
	nextBraceIsFunc := false

	// currentSugar reports the "return nil" replacement for the func body we are
	// currently inside, or "" if it doesn't return an element type. Walks inward
	// from the top of the stack to find the nearest enclosing func brace.
	currentSugar := func() string {
		for i := len(braceStack) - 1; i >= 0; i-- {
			if braceStack[i] {
				return sugarStack[i]
			}
		}
		return ""
	}

	// sugarBetweenLastFuncAndHere checks the source text between the most recent
	// "func" keyword and the current position for an element return type.
	// Used to infer whether a func body whose '{' we are about to consume returns an element.
	sugarBetweenLastFuncAndHere := func() string {
		end := s.pos
		funcIdx := strings.LastIndex(s.src[:end], "func")
		if funcIdx == -1 {
			return ""
		}
		return elementSugar(s.src[funcIdx:end])
	}

	for !s.atEnd() {
		c := s.cur()
		if c == '"' || c == '\'' || c == '`' {
			out.WriteString(s.readString())
			continue
		}
		if c == '/' && s.peek(1) == '/' {
			out.WriteString(s.readLineComment())
			continue
		}
		if c == '/' && s.peek(1) == '*' {
			out.WriteString(s.readBlockComment())
			continue
		}
		if c == '{' {
			sugar := ""
			if nextBraceIsFunc {
				sugar = sugarBetweenLastFuncAndHere()
			}
			braceStack = append(braceStack, nextBraceIsFunc)
			sugarStack = append(sugarStack, sugar)
			nextBraceIsFunc = false
			out.WriteByte(c)
			s.pos++
			continue
		}
		if c == '}' {
			if len(braceStack) > 0 {
				braceStack = braceStack[:len(braceStack)-1]
				sugarStack = sugarStack[:len(sugarStack)-1]
			}
			out.WriteByte(c)
			s.pos++
			continue
		}
		if c == 'f' && s.isKeyword("func") {
			nextBraceIsFunc = true
			out.WriteString("func")
			s.pos += len("func")
			continue
		}
		if c == 'r' && s.isKeyword("return") {
			result, wasVane, err := s.handleReturn(currentSugar())
			if err != nil {
				return "", err
			}
			out.WriteString(result)
			// Re-sync //line after vane syntax expansion so subsequent errors map to the right source line.
			if wasVane && s.filename != "" {
				fmt.Fprintf(&out, "//line %s:%d\n", s.filename, s.lineNum(s.pos))
			}
			continue
		}
		if c == '<' && s.looksLikeVane() {
			return "", s.errorf(s.pos,
				"vane syntax element outside return statement",
				"vane syntax is only valid inside return ( ... ) blocks.\n\n"+
					"  // Extract to a named function:\n"+
					"  func myEl() js.Value {\n"+
					"      return (<tag>...</tag>)\n"+
					"  }")
		}
		out.WriteByte(c)
		s.pos++
	}
	return out.String(), nil
}

func isVaneTagStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func (s *scanner) handleReturn(nilSugar string) (string, bool, error) {
	returnPos := s.pos
	s.pos += len("return")
	s.skipWS()

	isVaneStart := func() bool {
		return !s.atEnd() && s.cur() == '<' && (isVaneTagStart(s.peek(1)) || s.peek(1) == '>')
	}

	hasParen := false
	if !s.atEnd() && s.cur() == '(' {
		saved := s.pos
		s.pos++
		s.skipWS()
		if isVaneStart() {
			hasParen = true
		} else {
			s.pos = saved
		}
	}

	if isVaneStart() {
		node, err := s.parseElement()
		if err != nil {
			return "", false, err
		}
		if hasParen {
			s.skipWS()
			if !s.atEnd() && s.cur() == ')' {
				s.pos++
			}
		}
		em := &emitter{filename: s.filename, src: s.src}
		rootVar := em.emitNode(node, "")
		if em.err != nil {
			return "", false, em.err
		}
		tail := s.readUntilStatementEnd()

		lineDir := ""
		if s.filename != "" {
			lineDir = fmt.Sprintf("//line %s:%d\n", s.filename, lineAt(s.src, returnPos))
		}

		if tail != "" {
			return "\n" + lineDir + em.stmts.String() + "\treturn " + rootVar + tail + "\n", true, nil
		}

		return "\n" + lineDir + em.stmts.String() + "\treturn " + rootVar + "\n", true, nil
	}

	// "return nil" in .vane files is sugar for element-returning functions.
	// nilSugar is derived from the func signature, and only rewrites when inside a
	// func body whose return type is core.Node or js.Value, leaving func() func() etc. untouched.
	if nilSugar != "" && s.isKeyword("nil") {
		s.pos += len("nil")
		return "return " + nilSugar, false, nil
	}

	return "return ", false, nil
}

func (s *scanner) readUntilStatementEnd() string {
	start := s.pos

	depthParen := 0
	depthBrace := 0
	depthBracket := 0

	for !s.atEnd() {
		c := s.cur()

		switch c {
		case '"', '\'', '`':
			s.readString()
			continue

		case '(':
			depthParen++

		case ')':
			if depthParen > 0 {
				depthParen--
			}

		case '{':
			depthBrace++

		case '}':
			if depthBrace == 0 &&
				depthParen == 0 &&
				depthBracket == 0 {
				return strings.TrimSpace(s.src[start:s.pos])
			}

			if depthBrace > 0 {
				depthBrace--
			}

		case '[':
			depthBracket++

		case ']':
			if depthBracket > 0 {
				depthBracket--
			}

		case '\n':
			if depthParen == 0 &&
				depthBrace == 0 &&
				depthBracket == 0 {
				return strings.TrimSpace(s.src[start:s.pos])
			}
		}

		s.pos++
	}

	return strings.TrimSpace(s.src[start:s.pos])
}

//* vane syntax parser

func (s *scanner) parseElement() (*elemNode, error) {
	openPos := s.pos
	if s.atEnd() || s.cur() != '<' {
		return nil, s.errorf(s.pos, "expected '<'", "")
	}
	s.pos++

	// Fragment: <>...</>
	if !s.atEnd() && s.cur() == '>' {
		s.pos++ // consume '>'
		children, err := s.parseChildren("", openPos)
		if err != nil {
			return nil, err
		}
		return &elemNode{tag: "", pos: openPos, children: children}, nil
	}

	tag := s.readIdent()
	if tag == "" {
		return nil, s.errorf(openPos, fmt.Sprintf("expected tag name after '<', got %q", string(s.cur())), "")
	}

	var attrs []vaneAttr
	for {
		s.skipWS()
		if s.atEnd() || s.cur() == '>' || (s.cur() == '/' && s.peek(1) == '>') {
			break
		}
		a, err := s.parseAttr()
		if err != nil {
			return nil, wrapParseErr(fmt.Sprintf("in <%s>", tag), err)
		}
		attrs = append(attrs, a)
	}

	if s.cur() == '/' && s.peek(1) == '>' {
		s.pos += 2
		return &elemNode{tag: tag, pos: openPos, attrs: attrs}, nil
	}
	if s.cur() == '>' {
		s.pos++
	} else {
		return nil, s.errorf(s.pos, fmt.Sprintf("expected '>' or '/>' to close <%s>", tag), "")
	}

	children, err := s.parseChildren(tag, openPos)
	if err != nil {
		return nil, err
	}
	return &elemNode{tag: tag, pos: openPos, attrs: attrs, children: children}, nil
}

func (s *scanner) parseAttr() (vaneAttr, error) {
	namePos := s.pos
	name := s.readAttrName()
	if name == "" {
		return vaneAttr{}, s.errorf(namePos, fmt.Sprintf("expected attribute name, got %q", string(s.cur())), "")
	}
	s.skipWS()
	if s.atEnd() || s.cur() != '=' {
		return vaneAttr{name: name, namePos: namePos, value: "true", isExpr: true}, nil
	}
	s.pos++
	s.skipWS()
	if s.atEnd() {
		return vaneAttr{}, s.errorf(s.pos, fmt.Sprintf("expected value after %s=", name), "")
	}
	if s.cur() == '"' {
		valuePos := s.pos
		s.pos++
		start := s.pos
		for !s.atEnd() && s.cur() != '"' {
			if s.cur() == '\\' {
				s.pos++
			}
			s.pos++
		}
		val := s.src[start:s.pos]
		if !s.atEnd() {
			s.pos++
		}
		return vaneAttr{name: name, namePos: namePos, valuePos: valuePos, value: val, isExpr: false}, nil
	}
	if s.cur() == '{' {
		exprPos := s.pos
		s.pos++
		code, _, err := s.readGoExpr()
		if err != nil {
			return vaneAttr{}, s.errorf(exprPos, fmt.Sprintf("unterminated expression in attribute %s", name),
				"Make sure every '{' has a matching '}'.")
		}
		return vaneAttr{name: name, namePos: namePos, valuePos: exprPos, value: code, isExpr: true}, nil
	}
	return vaneAttr{}, s.errorf(s.pos, fmt.Sprintf("unexpected %q as value for attribute %s", string(s.cur()), name),
		"Use double quotes for string values: "+name+`="value", or curly braces for expressions: `+name+`={expr}`)
}

func (s *scanner) parseChildren(parentTag string, openPos int) ([]node, error) {
	var children []node
	var textBuf strings.Builder

	var textBufPos int
	flushText := func() {
		t := textBuf.String()
		leadLen := len(t) - len(strings.TrimLeft(t, " \t\n\r"))
		if strings.ContainsAny(t[:leadLen], "\n\r") {
			t = strings.TrimLeft(t, " \t\n\r")
		}
		t = strings.TrimRight(t, "\t\n\r")
		if t != "" {
			children = append(children, &textNode{content: t, pos: textBufPos})
		}
		textBuf.Reset()
	}

	for !s.atEnd() {
		if s.cur() == '<' && s.peek(1) == '/' {
			flushText()
			s.pos += 2
			closingPos := s.pos
			closing := s.readIdent()
			s.skipWS()
			if !s.atEnd() && s.cur() == '>' {
				s.pos++
			}
			if closing != parentTag {
				hint := ""
				if parentTag != "" {
					hint = fmt.Sprintf("The opening <%s> needs a matching </%s>.", parentTag, parentTag)
				}
				return nil, s.errorf(closingPos,
					fmt.Sprintf("expected </%s>, got </%s>", parentTag, closing),
					hint)
			}
			return children, nil
		}
		if s.cur() == '<' && (isVaneTagStart(s.peek(1)) || s.peek(1) == '>') {
			flushText()
			child, err := s.parseElement()
			if err != nil {
				return nil, err
			}
			children = append(children, child)
			continue
		}
		if s.cur() == '{' {
			flushText()
			exprPos := s.pos
			s.pos++
			code, spread, err := s.readGoExpr()
			if err != nil {
				return nil, s.errorf(exprPos, "unterminated Go expression: missing '}'",
					"Make sure every '{' in vane syntax child expressions has a matching '}'.")
			}
			if code != "" {
				trimmed := strings.TrimSpace(code)
				if isControlFlow(trimmed) {
					children = append(children, &ctrlFlowNode{raw: trimmed, pos: exprPos})
				} else {
					if strings.Contains(code, "func") && returnsElement(code) {
						// Bare uncalled func literal: {func() core.Node { ... }}. It's never called,
						// so DynChild receives a function value instead of a rendered element.
						if strings.HasSuffix(trimmed, "}") {
							return nil, s.errorf(exprPos,
								"func() core.Node literal is never called: result is not rendered",
								"Add () to invoke it: {func() core.Node { ... }()}")
						}
						recompiled, err := s.recompileExprFuncLiterals(code)
						if err != nil {
							return nil, err
						}
						code = recompiled
					}
					children = append(children, &exprNode{code: code, spread: spread, pos: exprPos})
				}
			}
			continue
		}
		// Only treat "//" and "/*" as comment markers when nothing but whitespace
		// has been buffered since the last flushed text/tag, i.e. a standalone
		// comment line between elements. A run of real text that happens to
		// contain "//" (a URL) or "/* */" (prose) is left alone.
		if s.cur() == '/' && s.peek(1) == '/' && strings.TrimSpace(textBuf.String()) == "" {
			for !s.atEnd() && s.cur() != '\n' {
				s.pos++
			}
			continue
		}
		if s.cur() == '/' && s.peek(1) == '*' && strings.TrimSpace(textBuf.String()) == "" {
			s.pos += 2
			for !s.atEnd() {
				if s.cur() == '*' && s.peek(1) == '/' {
					s.pos += 2
					break
				}
				s.pos++
			}
			continue
		}
		if textBuf.Len() == 0 {
			textBufPos = s.pos
		}
		textBuf.WriteByte(s.src[s.pos])
		s.pos++
	}
	if parentTag == "" {
		return nil, s.errorf(openPos, "unterminated fragment: missing closing </>", "")
	}
	return nil, s.errorf(openPos,
		fmt.Sprintf("unterminated element <%s>: missing </%s>", parentTag, parentTag),
		fmt.Sprintf("Add a closing </%s> tag.", parentTag))
}

//* Statement-based emitter
//
// Each call to handleReturn() creates a fresh emitter. The emitter accumulates
// Go statements in stmts and returns the variable name of the root element.
// Variable names _vane1, _vane2, … are local to each vane syntax block; since each
// block lives inside its own if/else/function body in Go, name collisions
// between blocks are fine.

type emitter struct {
	stmts     strings.Builder
	counter   int
	err       error
	filename  string
	src       string
	posOffset int // added to node positions when computing //line and error positions
}

func (em *emitter) errorf(pos int, msg, hint string) error {
	return &ParseError{filename: em.filename, src: em.src, pos: pos + em.posOffset, msg: msg, hint: hint}
}

// lineDir returns a //line directive string for pos, or "" if source mapping is disabled.
func (em *emitter) lineDir(pos int) string {
	if em.filename == "" || pos <= 0 {
		return ""
	}
	abs := pos + em.posOffset
	if abs < 0 || abs >= len(em.src) {
		return ""
	}
	return fmt.Sprintf("//line %s:%d\n", em.filename, lineAt(em.src, abs))
}

// bodyAbsStart finds the absolute byte offset of a ctrl-flow body string within src,
// searching in a window starting at searchFrom. Returns 0 if not found.
// body is a verbatim substring of src (extracted without transformation by ctrlReadBlock).
func bodyAbsStart(src string, searchFrom int, body string) int {
	if body == "" || searchFrom < 0 || searchFrom >= len(src) {
		return 0
	}
	end := searchFrom + 2*len(body) + 200
	if end > len(src) {
		end = len(src)
	}
	idx := strings.Index(src[searchFrom:end], body)
	if idx < 0 {
		return 0
	}
	return searchFrom + idx
}

func (em *emitter) fresh() string {
	em.counter++
	return fmt.Sprintf("_vane%d", em.counter)
}

// emitNode emits statements for node n. If parentVar != "", appends the result
// directly to parentVar and returns "". Otherwise returns the variable name.
func (em *emitter) emitNode(n node, parentVar string) string {
	switch v := n.(type) {
	case *elemNode:
		return em.emitElement(v, parentVar)
	case *ctrlFlowNode:
		return em.emitCtrlFlow(v, parentVar)
	case *textNode:
		if parentVar != "" {
			em.stmts.WriteString(em.lineDir(v.pos))
			fmt.Fprintf(&em.stmts, "\tcore.AppendText(%s, %q)\n", parentVar, v.content)
			return ""
		}
		v2 := em.fresh()
		em.stmts.WriteString(em.lineDir(v.pos))
		fmt.Fprintf(&em.stmts, "\t%s := core.Text(%q)\n", v2, v.content)
		return v2
	case *exprNode:
		return em.emitExpr(v, parentVar)
	}
	return ""
}

func (em *emitter) emitElement(el *elemNode, parentVar string) string {
	// Fragment: <></>
	// When nested, append children directly to parent (no wrapper node).
	// At root level, wrap in core.Fragment so the caller gets a single js.Value.
	if el.tag == "" {
		if parentVar != "" {
			for _, child := range el.children {
				em.emitNode(child, parentVar)
			}
			return ""
		}
		var childVars []string
		for _, child := range el.children {
			if v := em.emitNode(child, ""); v != "" {
				childVars = append(childVars, v)
			}
		}
		v := em.fresh()
		fmt.Fprintf(&em.stmts, "\t%s := core.Fragment(%s)\n", v, strings.Join(childVars, ", "))
		return v
	}

	// Components are Go function calls, not vane syntax tags: <Card title="x"/>
	// silently drops title and any children if allowed through, so this is a
	// compile error rather than the partial component-call it used to emit.
	if len(el.tag) > 0 && unicode.IsUpper(rune(el.tag[0])) {
		em.err = em.errorf(el.pos,
			fmt.Sprintf("`<%s/>` is not valid vane syntax: components are Go function calls", el.tag),
			fmt.Sprintf("call it as an expression instead: {%s()}", el.tag))
		return ""
	}

	v := em.fresh()
	em.stmts.WriteString(em.lineDir(el.pos))
	fmt.Fprintf(&em.stmts, "\t%s := core.El(%q)\n", v, el.tag)

	for _, attr := range el.attrs {
		em.emitAttr(v, attr)
		if em.err != nil {
			return ""
		}
	}
	for _, child := range el.children {
		em.emitNode(child, v)
	}

	if parentVar != "" {
		fmt.Fprintf(&em.stmts, "\tcore.AppendChild(%s, %s)\n", parentVar, v)
		return ""
	}
	return v
}

func (em *emitter) emitAttr(elVar string, attr vaneAttr) {
	attrPos := attr.valuePos
	if attrPos == 0 {
		attrPos = attr.namePos
	}
	ld := em.lineDir(attrPos)
	switch attr.name {
	case "key":
		em.stmts.WriteString(ld)
		if attr.isExpr {
			fmt.Fprintf(&em.stmts, "\tcore.Unwrap(%s).Set(\"key\", %s)\n", elVar, attr.value)
		} else {
			fmt.Fprintf(&em.stmts, "\tcore.Unwrap(%s).Set(\"key\", %q)\n", elVar, attr.value)
		}
	case "ref":
		if attr.isExpr {
			em.stmts.WriteString(ld)
			if strings.HasPrefix(strings.TrimSpace(attr.value), "&") {
				// Pointer form: ref={&myVar}, assign synchronously.
				fmt.Fprintf(&em.stmts, "\t*(%s) = %s\n", attr.value, elVar)
			} else {
				// Callback form: ref={func(el core.Node) { ... }}, called
				// synchronously with the element right when it's constructed.
				// Use this when code needs to run at that exact moment (e.g.
				// engaging a focus trap): a pointer ref plus a separate Effect
				// can't observe "the ref was just assigned" reliably, since
				// Effects only react to signal reads, not plain variable mutation.
				fmt.Fprintf(&em.stmts, "\t(%s)(%s)\n", attr.value, elVar)
			}
		}
	case "style":
		em.stmts.WriteString(ld)
		if !attr.isExpr {
			// style="color:red;font-size:14px", set cssText directly, no reactivity needed
			fmt.Fprintf(&em.stmts, "\tcore.Unwrap(%s).Get(\"style\").Set(\"cssText\", %q)\n", elVar, attr.value)
		} else {
			// style={core.Style{Color:"red"}}, reactive struct
			fmt.Fprintf(&em.stmts,
				"\tcore.DynStyle(%s, func() core.Style { return %s })\n",
				elVar, attr.value)
		}
	case "onClick":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnClick(%s, %s)\n", elVar, attr.value)
	case "onDblClick":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnDblClick(%s, %s)\n", elVar, attr.value)
	case "onInput":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnInput(%s, %s)\n", elVar, attr.value)
	case "onChange":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnChange(%s, %s)\n", elVar, attr.value)
	case "onChecked":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnChecked(%s, %s)\n", elVar, attr.value)
	case "onSubmit":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnSubmit(%s, %s)\n", elVar, attr.value)
	case "onKeyDown":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnKeyDown(%s, %s)\n", elVar, attr.value)
	case "onKeyUp":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnKeyUp(%s, %s)\n", elVar, attr.value)
	case "onBlur":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnBlur(%s, %s)\n", elVar, attr.value)
	case "onFocus":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnFocus(%s, %s)\n", elVar, attr.value)
	case "onMouseEnter":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnMouseEnter(%s, %s)\n", elVar, attr.value)
	case "onMouseLeave":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnMouseLeave(%s, %s)\n", elVar, attr.value)
	case "onScroll":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnScroll(%s, %s)\n", elVar, attr.value)
	case "onPointerDown":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnPointerDown(%s, %s)\n", elVar, attr.value)
	case "onPointerUp":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnPointerUp(%s, %s)\n", elVar, attr.value)
	case "onPointerMove":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnPointerMove(%s, %s)\n", elVar, attr.value)
	case "onTouchStart":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnTouchStart(%s, %s)\n", elVar, attr.value)
	case "onTouchEnd":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnTouchEnd(%s, %s)\n", elVar, attr.value)
	case "onDragStart":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnDragStart(%s, %s)\n", elVar, attr.value)
	case "onDrop":
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\tcore.OnDrop(%s, %s)\n", elVar, attr.value)
	default:
		// Unknown on* attributes are almost certainly a casing mistake (e.g. onclick
		// instead of onClick). Fail loudly instead of silently doing the wrong thing.
		if len(attr.name) > 2 && attr.name[:2] == "on" {
			suggested := "on" + strings.ToUpper(attr.name[2:3]) + attr.name[3:]
			em.err = em.errorf(attr.namePos,
				fmt.Sprintf("unknown event attribute %q", attr.name),
				fmt.Sprintf("Did you mean %s?", suggested))
			return
		}
		// A literal attribute name is almost always the HTML attribute spelling
		// typed out of habit (class, for, ...) rather than the DOM property vane
		// actually sets, and it's always a mistake since the property never gets
		// set. Catch it here, at compile time, since the name is static text a
		// human just typed; core.SetProp's own runtime check (core/props.go)
		// stays as the fallback for a dynamic key computed at runtime.
		if correct, ok := domattrs.Lookup(attr.name); ok {
			em.err = em.errorf(attr.namePos,
				fmt.Sprintf("%q is the HTML attribute spelling, not the DOM property Vane sets", attr.name),
				fmt.Sprintf("Did you mean %s?", correct))
			return
		}
		em.stmts.WriteString(ld)
		if attr.isExpr {
			fmt.Fprintf(&em.stmts,
				"\tcore.DynProp(%s, %q, func() any { return %s })\n",
				elVar, attr.name, attr.value)
		} else {
			fmt.Fprintf(&em.stmts,
				"\tcore.SetProp(%s, %q, %q)\n",
				elVar, attr.name, attr.value)
		}
	}
}

func (em *emitter) emitExpr(e *exprNode, parentVar string) string {
	ld := em.lineDir(e.pos)
	if e.spread {
		src := strings.TrimSpace(e.code)
		isFuncCall := strings.HasSuffix(src, ")")
		if parentVar != "" {
			em.stmts.WriteString(ld)
			if isFuncCall {
				// func() []core.Node, reactive list
				fmt.Fprintf(&em.stmts,
					"\tcore.DynList(%s, func() []core.Node { return %s })\n",
					parentVar, src)
			} else {
				// []core.Node / []any / variadic, static children spread
				fmt.Fprintf(&em.stmts,
					"\tcore.AppendChild(%s, core.Fragment(core.Spread(%s)...))\n",
					parentVar, src)
			}
			return ""
		}
		v := em.fresh()
		em.stmts.WriteString(ld)
		if isFuncCall {
			fmt.Fprintf(&em.stmts, "\t_ = %s // spread outside element\n", src)
		} else {
			fmt.Fprintf(&em.stmts, "\t%s := core.Fragment(core.Spread(%s)...)\n", v, src)
		}
		return v
	}

	if parentVar == "" {
		// Root-level expression (e.g. inside <> fragment with no parent var).
		// DynChild needs a real parent, so just evaluate and return the value.
		v := em.fresh()
		em.stmts.WriteString(ld)
		fmt.Fprintf(&em.stmts, "\t%s := %s\n", v, strings.TrimSpace(e.code))
		return v
	}

	em.stmts.WriteString(ld)
	fmt.Fprintf(&em.stmts,
		"\tcore.DynChild(%s, func() any { return %s })\n",
		parentVar, e.code)
	return ""
}

//* Inline control flow

func (em *emitter) emitCtrlFlow(n *ctrlFlowNode, parentVar string) string {
	em.stmts.WriteString(em.lineDir(n.pos))
	raw := n.raw
	switch {
	case strings.HasPrefix(raw, "for "):
		em.emitForCtrl(raw, parentVar, n.pos)
	case strings.HasPrefix(raw, "if "):
		em.emitIfCtrl(raw, parentVar, n.pos)
	case strings.HasPrefix(raw, "switch "), strings.HasPrefix(raw, "switch{"):
		em.emitSwitchCtrl(raw, parentVar, n.pos)
	default:
		em.err = fmt.Errorf("unrecognized control flow: %.30s", raw)
	}
	return ""
}

func (em *emitter) emitForCtrl(raw, parentVar string, basePos int) {
	after := strings.TrimPrefix(raw, "for ")
	header, rest, err := ctrlReadHeader(after)
	if err != nil {
		em.err = fmt.Errorf("for header: %w", err)
		return
	}
	body, _, err := ctrlReadBlock(rest)
	if err != nil {
		em.err = fmt.Errorf("for body: %w", err)
		return
	}
	if em.checkReturnInBody(body, "for", basePos) {
		return
	}
	parts, err := ctrlScanBody(body)
	if err != nil {
		em.err = fmt.Errorf("for body scan: %w", err)
		return
	}

	em.counter++
	listVar := fmt.Sprintf("_vaneItems%d", em.counter)
	subEm := &emitter{
		filename:  em.filename,
		src:       em.src,
		posOffset: bodyAbsStart(em.src, basePos+em.posOffset, body),
	}

	var out strings.Builder
	fmt.Fprintf(&out, "\tcore.DynList(%s, func() []core.Node {\n", parentVar)
	fmt.Fprintf(&out, "\t\tvar %s []core.Node\n", listVar)
	// Emit //line before the for header so VaneToGo maps the {for} vane line directly
	// to the for statement (not to the DynList wrapper above it). Without this,
	// linear interpolation lands 2 go-lines too late for the for header and body.
	if em.filename != "" {
		fmt.Fprintf(&out, "//line %s:%d\n", em.filename, lineAt(em.src, basePos+em.posOffset))
	}
	fmt.Fprintf(&out, "\t\tfor %s {\n", header)
	for _, p := range parts {
		if p.goCode != "" {
			for _, line := range strings.Split(p.goCode, "\n") {
				if t := strings.TrimSpace(line); t != "" {
					fmt.Fprintf(&out, "\t\t\t%s\n", t)
				}
			}
		}
		if p.vane != nil {
			subEm.stmts.Reset()
			elemVar := subEm.emitElement(p.vane, "")
			if subEm.err != nil {
				em.err = subEm.err
				return
			}
			for _, line := range strings.Split(subEm.stmts.String(), "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				if strings.HasPrefix(line, "//line ") {
					fmt.Fprintf(&out, "%s\n", line)
				} else {
					fmt.Fprintf(&out, "\t\t%s\n", line)
				}
			}
			if elemVar != "" {
				fmt.Fprintf(&out, "\t\t\t%s = append(%s, %s)\n", listVar, listVar, elemVar)
			}
		} else if p.callExpr != "" {
			// Emit //line for the call expression so GoToVane maps the append line
			// back to the correct vane source line (the call inside the for body).
			if em.filename != "" && subEm.posOffset > 0 {
				exprIdx := strings.Index(body, p.callExpr)
				if exprIdx >= 0 {
					lineNum := lineAt(em.src, subEm.posOffset+exprIdx)
					fmt.Fprintf(&out, "//line %s:%d\n", em.filename, lineNum)
				}
			}
			fmt.Fprintf(&out, "\t\t\t%s = append(%s, %s)\n", listVar, listVar, p.callExpr)
		}
	}
	out.WriteString("\t\t}\n")
	fmt.Fprintf(&out, "\t\treturn %s\n", listVar)
	out.WriteString("\t})\n")
	em.stmts.WriteString(out.String())
}

func (em *emitter) emitIfCtrl(raw, parentVar string, basePos int) {
	var out strings.Builder
	fmt.Fprintf(&out, "\tcore.DynChild(%s, func() any {\n", parentVar)

	rest := raw
	first := true

	for {
		rest = strings.TrimSpace(rest)
		if rest == "" {
			break
		}

		if first && strings.HasPrefix(rest, "if ") {
			rest = rest[3:]
			cond, blockRest, err := ctrlReadHeader(rest)
			if err != nil {
				em.err = fmt.Errorf("if cond: %w", err)
				return
			}
			body, remaining, err := ctrlReadBlock(blockRest)
			if err != nil {
				em.err = fmt.Errorf("if body: %w", err)
				return
			}
			if em.checkReturnInBody(body, "if", basePos) {
				return
			}
			parts, err := ctrlScanBody(body)
			if err != nil {
				em.err = fmt.Errorf("if scan: %w", err)
				return
			}
			fmt.Fprintf(&out, "\t\tif %s {\n", cond)
			em.emitBranchParts(parts, &out, bodyAbsStart(em.src, basePos+em.posOffset, body))
			rest = strings.TrimSpace(remaining)
			first = false
			continue
		}

		if !first && strings.HasPrefix(rest, "else if ") {
			rest = rest[8:]
			cond, blockRest, err := ctrlReadHeader(rest)
			if err != nil {
				em.err = fmt.Errorf("else if cond: %w", err)
				return
			}
			body, remaining, err := ctrlReadBlock(blockRest)
			if err != nil {
				em.err = fmt.Errorf("else if body: %w", err)
				return
			}
			if em.checkReturnInBody(body, "if", basePos) {
				return
			}
			parts, err := ctrlScanBody(body)
			if err != nil {
				em.err = fmt.Errorf("else if scan: %w", err)
				return
			}
			fmt.Fprintf(&out, "\t\t} else if %s {\n", cond)
			em.emitBranchParts(parts, &out, bodyAbsStart(em.src, basePos+em.posOffset, body))
			rest = strings.TrimSpace(remaining)
			continue
		}

		if !first && strings.HasPrefix(rest, "else") {
			rest = strings.TrimSpace(rest[4:])
			body, _, err := ctrlReadBlock(rest)
			if err != nil {
				em.err = fmt.Errorf("else body: %w", err)
				return
			}
			if em.checkReturnInBody(body, "if", basePos) {
				return
			}
			parts, err := ctrlScanBody(body)
			if err != nil {
				em.err = fmt.Errorf("else scan: %w", err)
				return
			}
			out.WriteString("\t\t} else {\n")
			em.emitBranchParts(parts, &out, bodyAbsStart(em.src, basePos+em.posOffset, body))
		}
		break
	}

	out.WriteString("\t\t}\n")
	out.WriteString("\t\treturn core.Empty()\n")
	out.WriteString("\t})\n")
	em.stmts.WriteString(out.String())
}

func (em *emitter) emitSwitchCtrl(raw, parentVar string, basePos int) {
	rest := strings.TrimSpace(strings.TrimPrefix(raw, "switch"))

	var expr string
	if !strings.HasPrefix(rest, "{") {
		var blockRest string
		var err error
		expr, blockRest, err = ctrlReadHeader(rest)
		if err != nil {
			em.err = fmt.Errorf("switch expr: %w", err)
			return
		}
		rest = blockRest
	}

	body, _, err := ctrlReadBlock(rest)
	if err != nil {
		em.err = fmt.Errorf("switch body: %w", err)
		return
	}

	switchBodyStart := bodyAbsStart(em.src, basePos+em.posOffset, body)

	cases, err := ctrlParseSwitchCases(body)
	if err != nil {
		em.err = fmt.Errorf("switch cases: %w", err)
		return
	}

	var out strings.Builder
	fmt.Fprintf(&out, "\tcore.DynChild(%s, func() any {\n", parentVar)
	if expr != "" {
		fmt.Fprintf(&out, "\t\tswitch %s {\n", expr)
	} else {
		out.WriteString("\t\tswitch {\n")
	}

	for _, c := range cases {
		if c.caseExpr == "" {
			out.WriteString("\t\tdefault:\n")
		} else {
			fmt.Fprintf(&out, "\t\tcase %s:\n", c.caseExpr)
		}
		if em.checkReturnInBody(c.body, "switch", basePos) {
			return
		}
		parts, err := ctrlScanBody(c.body)
		if err != nil {
			em.err = fmt.Errorf("case scan: %w", err)
			return
		}
		// caseBodyStart: absolute position of this case's body in em.src
		var caseBodyStart int
		if switchBodyStart > 0 {
			caseBodyStart = switchBodyStart + c.bodyStart
		}
		em.emitBranchParts(parts, &out, caseBodyStart)
	}

	out.WriteString("\t\t}\n")
	out.WriteString("\t\treturn core.Empty()\n")
	out.WriteString("\t})\n")
	em.stmts.WriteString(out.String())
}

// findReturnVanePos scans em.src from basePos forward for a `return (` or `return <`
// statement (the invalid pattern inside {for}/{if}/{switch} blocks).
// Returns the absolute byte offset of `return` in em.src, or basePos if not found.
func (em *emitter) findReturnVanePos(basePos int) int {
	if basePos < 0 || basePos >= len(em.src) {
		return basePos
	}
	src := em.src[basePos:]
	i := 0
	for i < len(src) {
		idx := strings.Index(src[i:], "return")
		if idx < 0 {
			return basePos
		}
		i += idx
		end := i + 6
		// must be a keyword boundary, so next char must not be a letter/underscore
		if end < len(src) {
			next := src[end]
			if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') || next == '_' {
				i = end
				continue
			}
		}
		after := strings.TrimLeft(src[end:], " \t")
		if len(after) > 0 && (after[0] == '(' || after[0] == '<') {
			return basePos + i
		}
		i = end
	}
	return basePos
}

// checkReturnInBody detects `return (` / `return <` in a control-flow body and sets em.err.
// Returns true if an error was set (caller should return immediately).
func (em *emitter) checkReturnInBody(body, keyword string, basePos int) bool {
	if !strings.Contains(body, "return") {
		return false
	}
	// Scan with brace-depth tracking. Only flag `return (<` at depth 0,
	// a `return` inside a nested func literal (depth > 0) is valid Go.
	depth := 0
	i := 0
	for i < len(body) {
		// Skip string literals.
		if body[i] == '"' || body[i] == '`' {
			quote := body[i]
			i++
			for i < len(body) && body[i] != quote {
				if body[i] == '\\' && quote == '"' {
					i++ // skip escaped char
				}
				i++
			}
			i++
			continue
		}
		// Skip line comments.
		if i+1 < len(body) && body[i] == '/' && body[i+1] == '/' {
			for i < len(body) && body[i] != '\n' {
				i++
			}
			continue
		}
		// Skip block comments.
		if i+1 < len(body) && body[i] == '/' && body[i+1] == '*' {
			i += 2
			for i+1 < len(body) && (body[i] != '*' || body[i+1] != '/') {
				i++
			}
			i += 2
			continue
		}
		if body[i] == '{' {
			depth++
			i++
			continue
		}
		if body[i] == '}' {
			if depth > 0 {
				depth--
			}
			i++
			continue
		}
		// Only flag `return` at depth 0 (directly in the ctrl-flow body,
		// not inside a nested func literal or other braced block).
		if depth == 0 && strings.HasPrefix(body[i:], "return") {
			end := i + 6
			if end < len(body) {
				next := body[end]
				if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') || next == '_' {
					i = end
					continue
				}
			}
			after := strings.TrimLeft(body[end:], " \t")
			if len(after) > 0 && (after[0] == '(' || after[0] == '<') {
				pos := em.findReturnVanePos(basePos)
				em.err = em.errorf(pos,
					fmt.Sprintf("`return` is not valid inside {%s} blocks: write vane syntax directly", keyword),
					"remove `return` and the surrounding parens:\n\t<tag>...</tag>\nfor complex conditional logic, use an IIFE: {func() js.Value { return (<tag/>) }()}")
				return true
			}
		}
		i++
	}
	return false
}

// emitBranchParts writes Go setup code + vane syntax return for an if/switch branch.
// bodyStart is the absolute byte offset of the branch body in em.src (0 = suppress //line).
func (em *emitter) emitBranchParts(parts []bodyPart, out *strings.Builder, bodyStart int) {
	subEm := &emitter{
		filename:  em.filename,
		src:       em.src,
		posOffset: bodyStart,
	}
	for _, p := range parts {
		if p.goCode != "" {
			for _, line := range strings.Split(p.goCode, "\n") {
				if t := strings.TrimSpace(line); t != "" {
					fmt.Fprintf(out, "\t\t\t%s\n", t)
				}
			}
		}
		if p.vane != nil {
			subEm.stmts.Reset()
			elemVar := subEm.emitElement(p.vane, "")
			if subEm.err != nil {
				em.err = subEm.err
				return
			}
			for _, line := range strings.Split(subEm.stmts.String(), "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				if strings.HasPrefix(line, "//line ") {
					fmt.Fprintf(out, "%s\n", line)
				} else {
					fmt.Fprintf(out, "\t\t%s\n", line)
				}
			}
			if elemVar != "" {
				fmt.Fprintf(out, "\t\t\treturn %s\n", elemVar)
			}
		} else if p.callExpr != "" {
			fmt.Fprintf(out, "\t\t\treturn %s\n", p.callExpr)
		}
	}
}

// ctrlReadHeader scans src up to the first unbalanced '{', returning
// (trimmed header, rest starting at '{').
func ctrlReadHeader(src string) (header, rest string, err error) {
	s := &scanner{src: src}
	depth := 0
	for !s.atEnd() {
		c := s.cur()
		if c == '"' || c == '\'' || c == '`' {
			s.readString()
			continue
		}
		if c == '/' && s.peek(1) == '/' {
			s.readLineComment()
			continue
		}
		if c == '/' && s.peek(1) == '*' {
			s.readBlockComment()
			continue
		}
		if c == '(' {
			depth++
			s.pos++
			continue
		}
		if c == ')' {
			depth--
			s.pos++
			continue
		}
		if c == '{' && depth == 0 {
			return strings.TrimSpace(src[:s.pos]), src[s.pos:], nil
		}
		s.pos++
	}
	return "", "", fmt.Errorf("no opening block brace found")
}

// ctrlReadBlock reads a {...} block from the start of src (src must start with '{').
// Returns (body without braces, rest after closing '}').
func ctrlReadBlock(src string) (body, rest string, err error) {
	src = strings.TrimSpace(src)
	if len(src) == 0 || src[0] != '{' {
		return "", "", fmt.Errorf("expected '{', got: %.10s", src)
	}
	s := &scanner{src: src, pos: 1}
	depth := 1
	start := 1
	for !s.atEnd() {
		c := s.cur()
		if c == '"' || c == '\'' || c == '`' {
			s.readString()
			continue
		}
		if c == '/' && s.peek(1) == '/' {
			s.readLineComment()
			continue
		}
		if c == '/' && s.peek(1) == '*' {
			s.readBlockComment()
			continue
		}
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start:s.pos], src[s.pos+1:], nil
			}
		}
		s.pos++
	}
	return "", "", fmt.Errorf("unterminated block")
}

// looksLikeCallExpr returns true if s looks like a bare function-call expression
// (ends with ')', not an assignment, not a keyword statement).
func looksLikeCallExpr(s string) bool {
	if !strings.HasSuffix(s, ")") {
		return false
	}
	for _, kw := range []string{"if ", "for ", "switch ", "return ", "go ", "defer ", "select "} {
		if strings.HasPrefix(s, kw) {
			return false
		}
	}
	firstParen := strings.IndexByte(s, '(')
	if firstParen < 0 {
		return false
	}
	before := s[:firstParen]
	return !strings.Contains(before, ":=") && !strings.Contains(before, " = ")
}

// extractLastExpr splits goCode into (setup, lastExpr) where lastExpr is the
// final depth-0 expression statement. Returns ("", "") if no suitable expr found.
func extractLastExpr(code string) (setup, expr string) {
	trimmed := strings.TrimRight(code, " \t\n\r")
	if trimmed == "" {
		return "", ""
	}

	depth := 0
	inStr := byte(0)
	lastBoundary := -1

	for i := 0; i < len(trimmed); i++ {
		c := trimmed[i]
		if inStr != 0 {
			if c == '\\' && inStr != '`' {
				i++
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = c
		case '/':
			if i+1 < len(trimmed) {
				if trimmed[i+1] == '/' {
					for i < len(trimmed) && trimmed[i] != '\n' {
						i++
					}
					continue
				} else if trimmed[i+1] == '*' {
					i += 2
					for i+1 < len(trimmed) && (trimmed[i] != '*' || trimmed[i+1] != '/') {
						i++
					}
					i++
					continue
				}
			}
		case '(', '{', '[':
			depth++
		case ')', '}', ']':
			depth--
		case '\n':
			if depth == 0 {
				lastBoundary = i + 1
			}
		}
	}

	var candidate string
	if lastBoundary < 0 || lastBoundary >= len(trimmed) {
		candidate = strings.TrimSpace(trimmed)
		if looksLikeCallExpr(candidate) {
			return "", candidate
		}
		return trimmed, ""
	}
	candidate = strings.TrimSpace(trimmed[lastBoundary:])
	if !looksLikeCallExpr(candidate) {
		return trimmed, ""
	}
	return trimmed[:lastBoundary], candidate
}

// ctrlScanBody parses a block body string, separating Go code from vane syntax elements.
// A '<' is treated as a vane syntax element only when not in a comparison context
// (i.e., not immediately following an identifier, ')', ']', or quote on the same line).
func ctrlScanBody(src string) ([]bodyPart, error) {
	s := &scanner{src: src}
	var parts []bodyPart
	var goBuf strings.Builder
	var lastSig byte = '\n'

	flush := func() {
		if g := goBuf.String(); strings.TrimSpace(g) != "" {
			parts = append(parts, bodyPart{goCode: g})
		}
		goBuf.Reset()
	}

	isCompCtx := func() bool {
		c := lastSig
		return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == ')' || c == ']' ||
			c == '"' || c == '\'' || c == '`' || c == '('
	}

	for !s.atEnd() {
		c := s.cur()
		if c == '"' || c == '\'' || c == '`' {
			str := s.readString()
			goBuf.WriteString(str)
			if len(str) > 0 {
				lastSig = str[len(str)-1]
			}
			continue
		}
		if c == '/' && s.peek(1) == '/' {
			goBuf.WriteString(s.readLineComment())
			continue
		}
		if c == '/' && s.peek(1) == '*' {
			goBuf.WriteString(s.readBlockComment())
			continue
		}
		if c == '<' && !isCompCtx() && (isVaneTagStart(s.peek(1)) || s.peek(1) == '>') {
			flush()
			prevSig := lastSig
			elem, err := s.parseElement()
			if err != nil {
				// If '<' was after a newline the user may have intended a comparison operator
				// that wrapped onto a new line (e.g., `x\n< y`). Add a clarifying hint.
				if prevSig == '\n' {
					if pe, ok := err.(*ParseError); ok {
						suffix := "\nIf '<' is a comparison operator, keep it on the same line as the left operand: `x < y`."
						if pe.hint == "" {
							pe.hint = suffix[1:] // trim leading newline
						} else {
							pe.hint += suffix
						}
					}
				}
				return nil, err
			}
			parts = append(parts, bodyPart{vane: elem})
			lastSig = '>'
			continue
		}
		if c == '\n' || c == '\r' {
			lastSig = '\n'
		} else if c != ' ' && c != '\t' {
			lastSig = c
		}
		goBuf.WriteByte(c)
		s.pos++
	}
	flush()

	// Post-process: if the last part has no vane syntax element, check whether its
	// final statement is a bare function-call expression returning js.Value (e.g. component calls).
	if len(parts) > 0 {
		last := &parts[len(parts)-1]
		if last.vane == nil && last.callExpr == "" {
			setup, expr := extractLastExpr(last.goCode)
			if expr != "" {
				last.goCode = setup
				last.callExpr = expr
			}
		}
	}

	return parts, nil
}

// ctrlParseSwitchCases splits a switch body into case/default segments.
func ctrlParseSwitchCases(src string) ([]switchCaseData, error) {
	s := &scanner{src: src}
	var cases []switchCaseData
	var currentExpr string
	bodyStart := 0
	inCase := false

	prevIsWord := func() bool {
		if s.pos == 0 {
			return false
		}
		p := s.src[s.pos-1]
		return (p >= 'a' && p <= 'z') || (p >= 'A' && p <= 'Z') || (p >= '0' && p <= '9') || p == '_'
	}

	for !s.atEnd() {
		c := s.cur()
		if c == '"' || c == '\'' || c == '`' {
			s.readString()
			continue
		}
		if c == '/' && s.peek(1) == '/' {
			s.readLineComment()
			continue
		}
		if c == '/' && s.peek(1) == '*' {
			s.readBlockComment()
			continue
		}

		if !prevIsWord() && s.isKeyword("case") {
			if inCase {
				cases = append(cases, switchCaseData{caseExpr: currentExpr, body: src[bodyStart:s.pos], bodyStart: bodyStart})
			}
			s.pos += 4
			s.skipWS()
			start := s.pos
			for !s.atEnd() {
				cc := s.cur()
				if cc == '"' || cc == '\'' || cc == '`' {
					s.readString()
					continue
				}
				if cc == ':' {
					break
				}
				s.pos++
			}
			currentExpr = strings.TrimSpace(src[start:s.pos])
			if !s.atEnd() {
				s.pos++ // skip ':'
			}
			bodyStart = s.pos
			inCase = true
			continue
		}

		if !prevIsWord() && s.isKeyword("default") {
			if inCase {
				cases = append(cases, switchCaseData{caseExpr: currentExpr, body: src[bodyStart:s.pos], bodyStart: bodyStart})
			}
			s.pos += 7
			s.skipWS()
			if !s.atEnd() && s.cur() == ':' {
				s.pos++
			}
			currentExpr = ""
			bodyStart = s.pos
			inCase = true
			continue
		}

		s.pos++
	}

	if inCase {
		cases = append(cases, switchCaseData{caseExpr: currentExpr, body: src[bodyStart:], bodyStart: bodyStart})
	}
	return cases, nil
}
