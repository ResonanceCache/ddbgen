// Package analyze implements ddbgen's generate-time static checks. Each
// check carries a stable error code (DDB001..DDB008), documented in
// docs/checks.md. Checks are conservative: when template overlap cannot be
// ruled out, they error with an explanation and a suggested fix.
package analyze

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ResonanceCache/ddbgen/internal/keytmpl"
	"github.com/ResonanceCache/ddbgen/internal/schema"
)

// Issue is one static-check finding.
type Issue struct {
	Code string
	Pos  schema.Pos
	Msg  string
}

func (i Issue) Error() string {
	return fmt.Sprintf("%s: %s: %s", i.Pos, i.Code, i.Msg)
}

// Issues renders a list of issues as a multi-line error.
type Issues []Issue

func (is Issues) Error() string {
	msgs := make([]string, len(is))
	for i, issue := range is {
		msgs[i] = issue.Error()
	}
	return strings.Join(msgs, "\n")
}

// Schema runs all checks and returns the findings sorted by position.
func Schema(s *schema.Schema) Issues {
	var out Issues
	for _, t := range s.Tables {
		out = append(out, checkTable(t)...)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Pos.File != b.Pos.File {
			return a.Pos.File < b.Pos.File
		}
		if a.Pos.Line != b.Pos.Line {
			return a.Pos.Line < b.Pos.Line
		}
		return a.Code < b.Code
	})
	return out
}

func checkTable(t *schema.Table) Issues {
	var out Issues
	for _, e := range t.Entities {
		out = append(out, checkPlaceholders(e)...)  // DDB007 + DDB004
		out = append(out, checkVersionTTL(e)...)    // DDB005
		out = append(out, checkSortability(e)...)   // DDB003
		out = append(out, checkPatterns(t, e)...)   // DDB002 (+ DDB003 for range values)
		out = append(out, checkReservedAttrs(e)...) // DDB008
	}
	out = append(out, checkCollisions(t)...) // DDB001
	out = append(out, checkDuplicates(t)...) // DDB006
	return out
}

// --- DDB008 reserved attribute names ---

// checkReservedAttrs rejects field attribute names that collide with the
// synthesized physical key attributes or the entity-type attribute:
// marshal overwrites those attributes after MarshalMap, so a colliding
// field would be silently clobbered on write and corrupted on read.
func checkReservedAttrs(e *schema.Entity) Issues {
	reserved := map[string]string{
		"pk":     "the physical partition-key attribute",
		e.ETAttr: "the entity-type attribute",
	}
	if e.Key.SK != nil {
		reserved["sk"] = "the physical sort-key attribute"
	}
	for _, ix := range e.Indexes {
		reserved[schema.PKAttrFor(ix.Name)] = "the physical partition-key attribute of GSI " + ix.Name
		if ix.Key.SK != nil {
			reserved[schema.SKAttrFor(ix.Name)] = "the physical sort-key attribute of GSI " + ix.Name
		}
	}
	var out Issues
	attrs := map[string]string{}
	for _, f := range e.Fields {
		if what, hit := reserved[f.Attr]; hit {
			out = append(out, Issue{
				Code: "DDB008", Pos: e.Pos,
				Msg: fmt.Sprintf("entity %s: field %s maps to attribute %q, which is %s; the synthesized value would silently overwrite the field on every write — pick another dynamodbav name",
					e.Name, f.Name, f.Attr, what),
			})
		}
		if prev, dup := attrs[f.Attr]; dup {
			out = append(out, Issue{
				Code: "DDB008", Pos: e.Pos,
				Msg: fmt.Sprintf("entity %s: fields %s and %s both map to attribute %q; one write would silently win",
					e.Name, prev, f.Name, f.Attr),
			})
		} else {
			attrs[f.Attr] = f.Name
		}
	}
	return out
}

// --- DDB007 placeholder resolution + DDB004 encoder/type compatibility ---

// templatesOf yields every key template of the entity with a description
// and the marker position it came from.
func templatesOf(e *schema.Entity) []struct {
	desc string
	tm   *keytmpl.Template
	pos  schema.Pos
} {
	var out []struct {
		desc string
		tm   *keytmpl.Template
		pos  schema.Pos
	}
	add := func(desc string, tm *keytmpl.Template, pos schema.Pos) {
		if tm != nil {
			out = append(out, struct {
				desc string
				tm   *keytmpl.Template
				pos  schema.Pos
			}{desc, tm, pos})
		}
	}
	add("pk", e.Key.PK, e.KeyPos)
	add("sk", e.Key.SK, e.KeyPos)
	for _, ix := range e.Indexes {
		add(ix.Name+" pk", ix.Key.PK, ix.Pos)
		if ix.Key.SK != nil {
			add(ix.Name+" sk", ix.Key.SK, ix.Pos)
		}
	}
	return out
}

func checkPlaceholders(e *schema.Entity) Issues {
	var out Issues
	for _, t := range templatesOf(e) {
		for _, seg := range t.tm.Placeholders() {
			f, ok := e.Field(seg.Field)
			if !ok {
				out = append(out, Issue{
					Code: "DDB007",
					Pos:  t.pos,
					Msg: fmt.Sprintf("entity %s %s template %q: placeholder {%s} does not resolve to an exported, marshaled field (unexported fields and dynamodbav:\"-\" fields cannot feed keys)",
						e.Name, t.desc, t.tm.Raw, seg.Field),
				})
				continue
			}
			if msg := encoderCompat(seg.Encoder, f.GoType); msg != "" {
				out = append(out, Issue{
					Code: "DDB004",
					Pos:  t.pos,
					Msg: fmt.Sprintf("entity %s %s template %q: placeholder {%s}: %s",
						e.Name, t.desc, t.tm.Raw, seg.Field, msg),
				})
			}
		}
	}
	return out
}

// encoderCompat returns "" when the encoder accepts the Go type, or an
// explanation.
func encoderCompat(encoder, goType string) string {
	ok := false
	var want string
	switch encoder {
	case "":
		ok, want = goType == "string", "string"
	case "rfc3339":
		ok, want = goType == "time.Time", "time.Time"
	case "epoch", "epochms":
		ok, want = goType == "time.Time" || goType == "int64", "time.Time or int64"
	case "upper", "lower", "ulid", "urlenc":
		ok, want = goType == "string", "string"
	case "hex":
		ok = goType == "[]byte" || isByteArray(goType)
		want = "[]byte or [N]byte"
	default:
		if keytmpl.PadWidth(encoder) > 0 {
			ok = goType == "int64" || strings.HasPrefix(goType, "uint")
			want = "int64 or an unsigned integer"
		} else {
			return fmt.Sprintf("unknown encoder %q", encoder)
		}
	}
	if ok {
		return ""
	}
	enc := encoder
	if enc == "" {
		enc = "(none)"
	}
	return fmt.Sprintf("encoder %s requires %s, but field is %s", enc, want, goType)
}

func isByteArray(goType string) bool {
	_, ok := keytmpl.FixedWidth("hex", goType)
	return ok && goType != "[]byte"
}

// --- DDB005 version/ttl typing ---

func checkVersionTTL(e *schema.Entity) Issues {
	var out Issues
	if e.VersionField != "" {
		f, ok := e.Field(e.VersionField)
		switch {
		case !ok:
			out = append(out, Issue{
				Code: "DDB005", Pos: e.Pos,
				Msg: fmt.Sprintf("entity %s: version field %s does not resolve to an exported, marshaled field", e.Name, e.VersionField),
			})
		case f.GoType != "int" && f.GoType != "int32" && f.GoType != "int64":
			out = append(out, Issue{
				Code: "DDB005", Pos: e.Pos,
				Msg: fmt.Sprintf("entity %s: version field %s must be an integer type (int, int32, int64), got %s", e.Name, e.VersionField, f.GoType),
			})
		}
	}
	if e.TTLField != "" {
		f, ok := e.Field(e.TTLField)
		switch {
		case !ok:
			out = append(out, Issue{
				Code: "DDB005", Pos: e.Pos,
				Msg: fmt.Sprintf("entity %s: ttl field %s does not resolve to an exported, marshaled field", e.Name, e.TTLField),
			})
		case f.GoType != "int64":
			out = append(out, Issue{
				Code: "DDB005", Pos: e.Pos,
				Msg: fmt.Sprintf("entity %s: ttl field %s must be int64 (unix seconds), got %s", e.Name, e.TTLField, f.GoType),
			})
		}
	}
	// Neither field may feed a key template: the version changes on every
	// update without recomputing keys (the index would silently drift), and
	// a TTL field in a key makes items unaddressable as expiry approaches.
	for _, t := range templatesOf(e) {
		for _, seg := range t.tm.Placeholders() {
			if e.VersionField != "" && seg.Field == e.VersionField {
				out = append(out, Issue{
					Code: "DDB005", Pos: t.pos,
					Msg: fmt.Sprintf("entity %s: version field %s is a placeholder in the %s template; every Update increments the version without recomputing keys, so the key would silently diverge from the data",
						e.Name, e.VersionField, t.desc),
				})
			}
			if e.TTLField != "" && seg.Field == e.TTLField {
				out = append(out, Issue{
					Code: "DDB005", Pos: t.pos,
					Msg: fmt.Sprintf("entity %s: ttl field %s is a placeholder in the %s template; TTL fields change over an item's life and must not feed keys",
						e.Name, e.TTLField, t.desc),
				})
			}
		}
	}
	return out
}

// --- DDB003 sortability ---

// checkSortability guards every place where lexicographic key order is
// relied upon for range semantics: bare range markers whose implicit cut
// placeholder is variable-width. (Valued range conditions are checked in
// checkPattern.) A non-final variable-width placeholder that no range
// touches is legal: equality ops and boundary-aligned begins_with cuts
// stay exact because the delimiter terminates every segment.
func checkSortability(e *schema.Entity) Issues {
	var out Issues
	for _, p := range e.Patterns {
		if p.SKValue != nil {
			continue
		}
		switch p.SKCond {
		case schema.SKBetween, schema.SKGt, schema.SKGte, schema.SKLt, schema.SKLte:
		default:
			continue
		}
		key, ok := e.KeyFor(p.Index)
		if !ok || key.SK == nil {
			continue // DDB002 reports these
		}
		cut := keytmpl.LeadingLiteralCut(key.SK)
		if cut.Next == nil {
			out = append(out, Issue{
				Code: "DDB003", Pos: p.Pos,
				Msg: fmt.Sprintf("pattern %s: declares sk.%s, but the sk template %q has no placeholder after its literal prefix to range over",
					p.Name, p.SKCond, key.SK.Raw),
			})
			continue
		}
		if !cut.RangeEligible(fieldTypes(e)) {
			out = append(out, Issue{
				Code: "DDB003", Pos: p.Pos,
				Msg: fmt.Sprintf("pattern %s: declares sk.%s over placeholder %s, whose encoding is variable-width, so lexicographic key order diverges from value order (a longer value can sort before a shorter one that is semantically smaller); use a fixed-width encoder (rfc3339, epoch, epochms, pad<N>, ulid, hex of [N]byte)",
					p.Name, p.SKCond, cut.Next),
			})
		}
	}
	return out
}

// --- DDB002 pattern satisfiability ---

func fieldTypes(e *schema.Entity) keytmpl.FieldTypes {
	return func(field string) (string, bool) {
		f, ok := e.Field(field)
		return f.GoType, ok
	}
}

func checkPatterns(t *schema.Table, e *schema.Entity) Issues {
	var out Issues
	for _, p := range e.Patterns {
		out = append(out, checkPattern(t, e, p)...)
	}
	return out
}

func checkPattern(t *schema.Table, e *schema.Entity, p *schema.Pattern) Issues {
	var out Issues
	key, ok := e.KeyFor(p.Index)
	if !ok {
		return Issues{{
			Code: "DDB002", Pos: p.Pos,
			Msg: fmt.Sprintf("pattern %s: entity %s declares no index %q", p.Name, e.Name, p.Index),
		}}
	}
	if p.Index != "main" {
		for _, ix := range t.Indexes {
			if ix.Name == p.Index && ix.Projection == "keys_only" {
				out = append(out, Issue{
					Code: "DDB002", Pos: p.Pos,
					Msg: fmt.Sprintf("pattern %s: GSI %s projects keys_only, so queried items carry no data attributes (and no entity-type attribute to filter on); typed pattern queries require projection=all",
						p.Name, p.Index),
				})
			}
		}
	}
	if !keytmpl.StructurallyEqual(p.PK, key.PK) {
		out = append(out, Issue{
			Code: "DDB002", Pos: p.Pos,
			Msg: fmt.Sprintf("pattern %s: pk template %q is not structurally identical to the pk template %q of %s on entity %s (same literals, fields, and encoders required, or the generated method would query keys that are never written)",
				p.Name, p.PK.Raw, key.PK.Raw, indexDesc(p.Index), e.Name),
		})
	}
	if p.SKCond == schema.SKNone && p.SKValue == nil {
		return out
	}
	if key.SK == nil {
		return append(out, Issue{
			Code: "DDB002", Pos: p.Pos,
			Msg: fmt.Sprintf("pattern %s: declares a sort-key condition, but %s of entity %s has no sort key", p.Name, indexDesc(p.Index), e.Name),
		})
	}
	if p.SKValue == nil {
		return out // bare between/gt/gte/lt/lte: refined through generated methods
	}
	cut, err := keytmpl.AlignPrefix(key.SK, p.SKValue, fieldTypes(e))
	if err != nil {
		return append(out, Issue{
			Code: "DDB002", Pos: p.Pos,
			Msg: fmt.Sprintf("pattern %s: %v", p.Name, err),
		})
	}
	switch p.SKCond {
	case schema.SKEq:
		if cut.Consumed != len(key.SK.Segments) || p.SKValue.TrailingDelim {
			out = append(out, Issue{
				Code: "DDB002", Pos: p.Pos,
				Msg: fmt.Sprintf("pattern %s: sk.eq value %q must specify the complete sort key %q; keys shorter than the template are never written, so a prefix equality can never match (use sk.begins for prefixes)",
					p.Name, p.SKValue.Raw, key.SK.Raw),
			})
		}
	case schema.SKBegins:
		if cut.Consumed == len(key.SK.Segments) && p.SKValue.TrailingDelim {
			out = append(out, Issue{
				Code: "DDB002", Pos: p.Pos,
				Msg: fmt.Sprintf("pattern %s: sk.begins value %q covers the whole sort key %q and ends with the delimiter; keys never end with a delimiter, so the condition can never match (drop the trailing %q)",
					p.Name, p.SKValue.Raw, key.SK.Raw, keytmpl.Delimiter),
			})
		}
	case schema.SKGt, schema.SKGte, schema.SKLt, schema.SKLte:
		segs := p.SKValue.Segments
		if len(segs) == 0 || segs[len(segs)-1].Kind != keytmpl.SegPlaceholder || p.SKValue.TrailingDelim {
			out = append(out, Issue{
				Code: "DDB002", Pos: p.Pos,
				Msg: fmt.Sprintf("pattern %s: sk.%s value %q must end with a placeholder (the range bound); a literal-terminated range does not cut at a placeholder boundary",
					p.Name, p.SKCond, p.SKValue.Raw),
			})
			break
		}
		last := segs[len(segs)-1]
		goType, _ := fieldTypes(e)(last.Field)
		if _, fixed := keytmpl.FixedWidth(last.Encoder, goType); !fixed {
			out = append(out, Issue{
				Code: "DDB003", Pos: p.Pos,
				Msg: fmt.Sprintf("pattern %s: sk.%s ranges over placeholder %s, whose encoding is variable-width; lexicographic comparison would not follow value order — use a fixed-width encoder",
					p.Name, p.SKCond, last),
			})
		}
	}
	return out
}

func indexDesc(index string) string {
	if index == "main" {
		return "the main index"
	}
	return "GSI " + index
}

// --- DDB001 key collision ---

// canOverlap reports whether two templates could ever render the same
// string. Delimiters never occur inside encoded segments, so templates
// with different segment counts are always distinct, and two literal
// segments at the same position must match exactly. Any position pairing a
// placeholder with anything is conservatively treated as overlapping.
func canOverlap(a, b *keytmpl.Template) bool {
	if len(a.Segments) != len(b.Segments) {
		return false
	}
	for i := range a.Segments {
		sa, sb := a.Segments[i], b.Segments[i]
		if sa.Kind == keytmpl.SegLiteral && sb.Kind == keytmpl.SegLiteral && sa.Literal != sb.Literal {
			return false
		}
	}
	return true
}

func checkCollisions(t *schema.Table) Issues {
	var out Issues
	indexes := []string{"main"}
	for _, ix := range t.Indexes {
		indexes = append(indexes, ix.Name)
	}
	for _, index := range indexes {
		for i := 0; i < len(t.Entities); i++ {
			for j := i + 1; j < len(t.Entities); j++ {
				a, b := t.Entities[i], t.Entities[j]
				ka, okA := a.KeyFor(index)
				kb, okB := b.KeyFor(index)
				if !okA || !okB {
					continue
				}
				if !canOverlap(ka.PK, kb.PK) {
					continue
				}
				skDisjoint := ka.SK != nil && kb.SK != nil && !canOverlap(ka.SK, kb.SK)
				if skDisjoint {
					continue
				}
				out = append(out, Issue{
					Code: "DDB001",
					Pos:  b.KeyPos,
					Msg: fmt.Sprintf("entities %s and %s can write items with identical keys on %s: pk %q vs %q overlap and the sort keys do not disambiguate (%s vs %s); add a distinguishing literal segment, e.g. an entity-name prefix, to one of the templates",
						a.Name, b.Name, indexDesc(index), ka.PK.Raw, kb.PK.Raw, rawOrNone(ka.SK), rawOrNone(kb.SK)),
				})
			}
		}
	}
	return out
}

func rawOrNone(t *keytmpl.Template) string {
	if t == nil {
		return "no sk"
	}
	return fmt.Sprintf("%q", t.Raw)
}

// --- DDB006 duplicate names ---

func checkDuplicates(t *schema.Table) Issues {
	var out Issues
	types := map[string]*schema.Entity{}
	patterns := map[string]*schema.Entity{}
	for _, e := range t.Entities {
		if prev, dup := types[e.Type]; dup {
			out = append(out, Issue{
				Code: "DDB006", Pos: e.Pos,
				Msg: fmt.Sprintf("entity type %q of %s duplicates entity %s (at %s); collection dispatch on the entity-type attribute would be ambiguous",
					e.Type, e.Name, prev.Name, prev.Pos),
			})
		} else {
			types[e.Type] = e
		}
		for _, p := range e.Patterns {
			if prev, dup := patterns[p.Name]; dup {
				out = append(out, Issue{
					Code: "DDB006", Pos: p.Pos,
					Msg: fmt.Sprintf("pattern name %s on entity %s duplicates a pattern on entity %s; generated method names would collide",
						p.Name, e.Name, prev.Name),
				})
			} else {
				patterns[p.Name] = e
			}
		}
	}
	return out
}
