package keytmpl

import (
	"fmt"
	"strings"
)

// Cut describes where a static sort-key condition stops within an sk
// template, and which placeholder (if any) range-refinement methods may
// legally target. Cuts exist only at placeholder boundaries: a condition
// may never split a placeholder's encoding.
type Cut struct {
	// Consumed is the number of leading template segments covered by the
	// static condition.
	Consumed int
	// Next is the first segment after the cut when it is a placeholder;
	// nil when the template is fully consumed or a literal follows.
	Next *Segment
	// NextIsFinal reports whether Next is the template's last segment.
	NextIsFinal bool
}

// FieldTypes resolves a placeholder field name to its Go type (for
// fixed-width decisions, e.g. hex over [N]byte).
type FieldTypes func(field string) (goType string, ok bool)

// AlignPrefix verifies that a condition prefix aligns to a segment boundary
// of the template and returns the resulting cut.
//
// Alignment rules: every prefix segment must structurally match the
// template segment at its position. A final literal prefix segment without
// a trailing delimiter must still match its template literal completely
// (a mid-literal cut like begins="ORD" is rejected: it also matches
// sibling literals and cannot anchor range bounds). A final placeholder
// prefix segment without a trailing delimiter is accepted only when its
// encoder is fixed-width: for variable-width encodings the condition would
// match mid-value (begins "PAY#o1" also matches order "o12").
func AlignPrefix(t *Template, p *Prefix, types FieldTypes) (*Cut, error) {
	if len(p.Segments) > len(t.Segments) {
		return nil, fmt.Errorf("condition %q has more segments than sk template %q", p.Raw, t.Raw)
	}
	for i, ps := range p.Segments {
		ts := t.Segments[i]
		last := i == len(p.Segments)-1
		if ps.Kind != ts.Kind {
			return nil, fmt.Errorf("condition %q segment %d (%s) does not match sk template %q segment (%s)",
				p.Raw, i+1, ps, t.Raw, ts)
		}
		if ps.Kind == SegPlaceholder {
			if ps.Field != ts.Field || ps.Encoder != ts.Encoder {
				return nil, fmt.Errorf("condition %q placeholder %s does not match sk template %q placeholder %s",
					p.Raw, ps, t.Raw, ts)
			}
			if last && !p.TrailingDelim && !(i == len(t.Segments)-1) {
				if _, fixed := placeholderWidth(ps, types); !fixed {
					return nil, fmt.Errorf("condition %q ends inside placeholder %s: %s is variable-width, so the condition can match part of a value; end the condition with %q or use a fixed-width encoder",
						p.Raw, ps, ps, Delimiter)
				}
			}
			continue
		}
		if ps.Literal != ts.Literal {
			if last && !p.TrailingDelim && strings.HasPrefix(ts.Literal, ps.Literal) {
				return nil, fmt.Errorf("condition %q ends mid-literal: %q is a partial match of template literal %q, which is not a placeholder boundary",
					p.Raw, ps.Literal, ts.Literal)
			}
			return nil, fmt.Errorf("condition %q literal %q does not match sk template %q literal %q",
				p.Raw, ps.Literal, t.Raw, ts.Literal)
		}
	}
	return cutAt(t, len(p.Segments)), nil
}

// LeadingLiteralCut returns the cut after the template's leading run of
// literal segments. Patterns without an explicit sk condition use it as an
// implicit entity-scoping begins_with prefix.
func LeadingLiteralCut(t *Template) *Cut {
	n := 0
	for n < len(t.Segments) && t.Segments[n].Kind == SegLiteral {
		n++
	}
	return cutAt(t, n)
}

// PrefixString renders the static string prefix for the first n segments
// of the template, ending with a delimiter when segments remain. Only
// valid when those segments are all literals.
func PrefixString(t *Template, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(t.Segments[i].Literal)
		b.WriteString(Delimiter)
	}
	if n == len(t.Segments) {
		return strings.TrimSuffix(b.String(), Delimiter)
	}
	return b.String()
}

func cutAt(t *Template, consumed int) *Cut {
	c := &Cut{Consumed: consumed}
	if consumed < len(t.Segments) && t.Segments[consumed].Kind == SegPlaceholder {
		c.Next = &t.Segments[consumed]
		c.NextIsFinal = consumed == len(t.Segments)-1
	}
	return c
}

func placeholderWidth(seg Segment, types FieldTypes) (int, bool) {
	goType := ""
	if types != nil {
		goType, _ = types(seg.Field)
	}
	return FixedWidth(seg.Encoder, goType)
}

// RangeEligible reports whether range-refinement methods may be generated
// for the cut's next placeholder: it must exist and be fixed-width, since
// lexicographic bounds over variable-width encodings do not follow value
// order.
func (c *Cut) RangeEligible(types FieldTypes) bool {
	if c.Next == nil {
		return false
	}
	_, fixed := placeholderWidth(*c.Next, types)
	return fixed
}
