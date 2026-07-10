// Package keytmpl implements the key template engine: parsing templates
// like "ORDER#{CreatedAt:rfc3339}#{OrderID}" into segments, encoding field
// values into key strings, decoding them back, and analyzing placeholder
// boundaries for range-query generation.
package keytmpl

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Delimiter joins template segments in physical key strings. Hardcoded in
// v1; per-table configuration is planned for v2.
const Delimiter = "#"

// SegmentKind distinguishes literal segments from placeholders.
type SegmentKind string

const (
	// SegLiteral is a fixed string segment.
	SegLiteral SegmentKind = "lit"
	// SegPlaceholder is a `{Field}` or `{Field:encoder}` segment.
	SegPlaceholder SegmentKind = "ph"
)

// Segment is one delimiter-separated part of a key template.
type Segment struct {
	Kind    SegmentKind `json:"kind"`
	Literal string      `json:"literal,omitempty"`
	Field   string      `json:"field,omitempty"`
	Encoder string      `json:"encoder,omitempty"`
}

// String renders the segment in template syntax.
func (s Segment) String() string {
	if s.Kind == SegLiteral {
		return s.Literal
	}
	if s.Encoder != "" {
		return "{" + s.Field + ":" + s.Encoder + "}"
	}
	return "{" + s.Field + "}"
}

// Template is a parsed key template.
type Template struct {
	Raw      string    `json:"raw"`
	Segments []Segment `json:"segments"`
}

// Prefix is a parsed sk-condition value: a template prefix that may end
// with a trailing delimiter (e.g. `ORDER#`).
type Prefix struct {
	Raw           string    `json:"raw"`
	Segments      []Segment `json:"segments,omitempty"`
	TrailingDelim bool      `json:"trailing_delim,omitempty"`
}

// ParseError reports a syntax error at a byte offset within the raw
// template string, so callers can position a caret in the source marker.
type ParseError struct {
	Offset int
	Msg    string
}

func (e *ParseError) Error() string { return e.Msg }

var (
	placeholderRe = regexp.MustCompile(`^\{([A-Za-z][A-Za-z0-9_]*)(?::([a-z][a-z0-9]*))?\}$`)
	encoderRe     = regexp.MustCompile(`^(rfc3339|epoch|epochms|upper|lower|hex|ulid|urlenc|pad([1-9][0-9]?))$`)
)

// Parse parses a key template. Every delimiter-separated part must be
// entirely a literal or entirely a placeholder; empty parts are rejected.
func Parse(raw string) (*Template, error) {
	segs, err := parseSegments(raw, false)
	if err != nil {
		return nil, err
	}
	return &Template{Raw: raw, Segments: segs}, nil
}

// ParsePrefix parses an sk-condition value, permitting a trailing
// delimiter that anchors the prefix at a segment boundary.
func ParsePrefix(raw string) (*Prefix, error) {
	if raw == "" || raw == Delimiter {
		return nil, &ParseError{Offset: 0, Msg: "empty condition value"}
	}
	trailing := strings.HasSuffix(raw, Delimiter)
	body := strings.TrimSuffix(raw, Delimiter)
	segs, err := parseSegments(body, true)
	if err != nil {
		return nil, err
	}
	return &Prefix{Raw: raw, Segments: segs, TrailingDelim: trailing}, nil
}

func parseSegments(raw string, allowEmpty bool) ([]Segment, error) {
	if raw == "" {
		if allowEmpty {
			return nil, nil
		}
		return nil, &ParseError{Offset: 0, Msg: "empty template"}
	}
	parts := strings.Split(raw, Delimiter)
	segs := make([]Segment, 0, len(parts))
	offset := 0
	for i, part := range parts {
		if i > 0 {
			offset++ // the delimiter
		}
		seg, err := parseSegment(part, offset)
		if err != nil {
			return nil, err
		}
		segs = append(segs, seg)
		offset += len(part)
	}
	return segs, nil
}

func parseSegment(part string, offset int) (Segment, error) {
	if part == "" {
		return Segment{}, &ParseError{Offset: offset, Msg: "empty segment: consecutive, leading, or trailing delimiters are not allowed"}
	}
	if !strings.ContainsAny(part, "{}") {
		return Segment{Kind: SegLiteral, Literal: part}, nil
	}
	m := placeholderRe.FindStringSubmatch(part)
	if m == nil {
		return Segment{}, &ParseError{Offset: offset, Msg: fmt.Sprintf("invalid segment %q: a segment must be a pure literal or a single {Field[:encoder]} placeholder", part)}
	}
	field, enc := m[1], m[2]
	if enc != "" && !encoderRe.MatchString(enc) {
		return Segment{}, &ParseError{
			Offset: offset + len(field) + 2, // past "{Field:"
			Msg:    fmt.Sprintf("unknown encoder %q (want rfc3339, epoch, epochms, pad<N>, upper, lower, hex, ulid, or urlenc)", enc),
		}
	}
	return Segment{Kind: SegPlaceholder, Field: field, Encoder: enc}, nil
}

// Placeholders returns the placeholder segments in order.
func (t *Template) Placeholders() []Segment {
	return placeholders(t.Segments)
}

// Placeholders returns the placeholder segments in order.
func (p *Prefix) Placeholders() []Segment {
	return placeholders(p.Segments)
}

func placeholders(segs []Segment) []Segment {
	var out []Segment
	for _, s := range segs {
		if s.Kind == SegPlaceholder {
			out = append(out, s)
		}
	}
	return out
}

// StructurallyEqual reports whether two templates have identical segment
// sequences: same literals, same fields, same encoders, same order.
func StructurallyEqual(a, b *Template) bool {
	if len(a.Segments) != len(b.Segments) {
		return false
	}
	for i := range a.Segments {
		if a.Segments[i] != b.Segments[i] {
			return false
		}
	}
	return true
}

// PadWidth returns N for a pad<N> encoder name, or 0.
func PadWidth(encoder string) int {
	if !strings.HasPrefix(encoder, "pad") {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimPrefix(encoder, "pad"))
	if err != nil {
		return 0
	}
	return n
}

// FixedWidth reports the encoded width of a placeholder when it is
// statically fixed, given the encoder and the Go type of the source field
// (hex is fixed-width only for [N]byte arrays).
func FixedWidth(encoder, goType string) (int, bool) {
	switch encoder {
	case "rfc3339":
		return 30, true // 2006-01-02T15:04:05.000000000Z
	case "epoch":
		return 12, true
	case "epochms":
		return 15, true
	case "ulid":
		return 26, true
	case "hex":
		if n, ok := byteArrayLen(goType); ok {
			return 2 * n, true
		}
		return 0, false
	}
	if n := PadWidth(encoder); n > 0 {
		return n, true
	}
	return 0, false
}

func byteArrayLen(goType string) (int, bool) {
	if !strings.HasPrefix(goType, "[") || !strings.HasSuffix(goType, "]byte") {
		return 0, false
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(goType, "["), "]byte")
	if inner == "" {
		return 0, false // []byte slice, variable length
	}
	n, err := strconv.Atoi(inner)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
