package parser

import (
	"fmt"
	"strings"
)

// Error is a marker diagnostic positioned in source. It renders as
// file:line:col followed by the marker line with a caret under the
// offending token.
type Error struct {
	File string
	Line int
	Col  int // 1-based column of the caret in the source line
	// Snippet is the marker comment text; CaretOff is the byte offset of
	// the offending token within it.
	Snippet  string
	CaretOff int
	Msg      string
}

func (e *Error) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s:%d:%d: %s", e.File, e.Line, e.Col, e.Msg)
	if e.Snippet != "" {
		b.WriteString("\n\t")
		b.WriteString(e.Snippet)
		b.WriteString("\n\t")
		b.WriteString(strings.Repeat(" ", e.CaretOff))
		b.WriteString("^")
	}
	return b.String()
}

// marker is one lexed //ddb: comment line.
type marker struct {
	directive string
	args      []arg
	file      string
	line      int
	col       int // column of the comment start in the source line
	text      string
}

// arg is one key[=value] pair in a marker.
type arg struct {
	key      string
	value    string
	hasValue bool
	keyOff   int // byte offset of key within marker text
	valOff   int // byte offset of value (inside quotes, if quoted)
}

func (m *marker) errAt(off int, format string, a ...any) *Error {
	return &Error{
		File:     m.file,
		Line:     m.line,
		Col:      m.col + off,
		Snippet:  m.text,
		CaretOff: off,
		Msg:      fmt.Sprintf(format, a...),
	}
}

const markerPrefix = "//ddb:"

// lexMarker splits a //ddb: comment line into a directive and key=value
// arguments, tracking byte offsets for caret diagnostics.
func lexMarker(m *marker) error {
	s := m.text
	i := len(markerPrefix)
	start := i
	for i < len(s) && isIdentChar(s[i]) {
		i++
	}
	if i == start {
		return m.errAt(start, "missing directive after //ddb: (want entity, key, index, or pattern)")
	}
	m.directive = s[start:i]
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			return nil
		}
		keyStart := i
		for i < len(s) && (isIdentChar(s[i]) || s[i] == '.') {
			i++
		}
		if i == keyStart {
			return m.errAt(i, "unexpected character %q (want key=value or a bare flag)", s[i])
		}
		a := arg{key: s[keyStart:i], keyOff: keyStart}
		if i < len(s) && s[i] == '=' {
			i++
			a.hasValue = true
			if i < len(s) && s[i] == '"' {
				i++
				a.valOff = i
				end := strings.IndexByte(s[i:], '"')
				if end < 0 {
					return m.errAt(i-1, "unterminated quoted value for %q", a.key)
				}
				a.value = s[i : i+end]
				i += end + 1
			} else {
				a.valOff = i
				valStart := i
				for i < len(s) && s[i] != ' ' && s[i] != '\t' {
					i++
				}
				a.value = s[valStart:i]
				if a.value == "" {
					return m.errAt(valStart, "missing value for %q", a.key)
				}
			}
		}
		m.args = append(m.args, a)
	}
}

func isIdentChar(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// argSet validates keys against the allowed set, rejects duplicates, and
// returns args indexed by key.
func (m *marker) argSet(allowed ...string) (map[string]arg, error) {
	ok := map[string]bool{}
	for _, k := range allowed {
		ok[k] = true
	}
	out := map[string]arg{}
	for _, a := range m.args {
		if !ok[a.key] {
			return nil, m.errAt(a.keyOff, "unknown %s argument %q (want one of: %s)", m.directive, a.key, strings.Join(allowed, ", "))
		}
		if _, dup := out[a.key]; dup {
			return nil, m.errAt(a.keyOff, "duplicate argument %q", a.key)
		}
		out[a.key] = a
	}
	return out, nil
}

// need returns the required valued argument key from set, or a positioned error.
func (m *marker) need(set map[string]arg, key string) (arg, error) {
	a, ok := set[key]
	if !ok {
		return arg{}, m.errAt(len(markerPrefix), "missing required argument %s=", key)
	}
	if !a.hasValue {
		return arg{}, m.errAt(a.keyOff, "argument %q requires a value", key)
	}
	return a, nil
}
