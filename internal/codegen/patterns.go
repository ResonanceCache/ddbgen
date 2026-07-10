package codegen

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ResonanceCache/ddbgen/internal/keytmpl"
	"github.com/ResonanceCache/ddbgen/internal/schema"
)

// rangeView describes the boundary-cut refinement methods of a query:
// <Base>After, <Base>Before, <Base>Between on the first placeholder past
// the static sk prefix.
type rangeView struct {
	Base      string // Created (from CreatedAt)
	ParamType string // time.Time
	EncHasErr bool
	Field     string // CreatedAt
	Encoder   string // rfc3339

	encFor  func(arg string) string
	predFor func(arg string) string
}

// Enc renders the encoding call for the given argument name.
func (r *rangeView) Enc(arg string) string { return r.encFor(arg) }

// Pred renders the predecessor call for the given argument name.
func (r *rangeView) Pred(arg string) string { return r.predFor(arg) }

// patternView drives one generated pattern query.
type patternView struct {
	Name      string // OrdersByTenant
	QueryType string // OrdersByTenantQuery
	Entity    string
	Client    string
	Index     string // "" for main
	PKAttr    string
	SKAttr    string // "" when the index has no sort key
	Params    []param
	PKFunc    string
	PKArgs    string

	// Static sk condition, assembled in the constructor.
	CondKind  string    // "", "implicit", "eq", "begins", "gt", "gte", "lt", "lte"
	CondSteps []encStep // encoding steps for placeholders in the condition value
	ValExpr   string    // Go expr for the full condition value
	PredCall  string    // for lt: predecessor call on the value's final placeholder
	PredFull  string    // for lt: Go expr for static part + pred
	ScopeQ    string    // quoted leading-literal scope of the entity's sk template

	Range         *rangeView
	HasSKStore    bool // query stores skPrefix for range methods
	HasCond       bool // query carries an SKCond
	HasConsistent bool // main index only
	Doc           string
}

func strip(field string) string {
	if s := strings.TrimSuffix(field, "At"); s != "" {
		return s
	}
	return field
}

// rangeFor builds the refinement view for a cut placeholder, or nil when
// the placeholder is not range-eligible.
func rangeFor(e *schema.Entity, cut *keytmpl.Cut) (*rangeView, error) {
	if cut.Next == nil {
		return nil, nil
	}
	f, ok := e.Field(cut.Next.Field)
	if !ok {
		return nil, fmt.Errorf("%s: sk template references unknown field {%s}", e.Pos, cut.Next.Field)
	}
	if _, fixed := keytmpl.FixedWidth(cut.Next.Encoder, f.GoType); !fixed {
		return nil, nil
	}
	rv := &rangeView{
		Base:      strip(f.Name),
		ParamType: f.GoType,
		Field:     f.Name,
		Encoder:   cut.Next.Encoder,
	}
	switch cut.Next.Encoder {
	case "rfc3339":
		rv.EncHasErr = true
		rv.encFor = func(a string) string { return "runtime.EncodeRFC3339(" + a + ")" }
		rv.predFor = func(a string) string { return "runtime.PredRFC3339(" + a + ")" }
	case "epoch":
		rv.EncHasErr = true
		if f.GoType == "time.Time" {
			rv.encFor = func(a string) string { return "runtime.EncodeEpochTime(" + a + ")" }
			rv.predFor = func(a string) string { return "runtime.PredEpoch(" + a + ".Unix())" }
		} else {
			rv.encFor = func(a string) string { return "runtime.EncodeEpoch(" + a + ")" }
			rv.predFor = func(a string) string { return "runtime.PredEpoch(" + a + ")" }
		}
	case "epochms":
		rv.EncHasErr = true
		if f.GoType == "time.Time" {
			rv.encFor = func(a string) string { return "runtime.EncodeEpochMSTime(" + a + ")" }
			rv.predFor = func(a string) string { return "runtime.PredEpochMS(" + a + ".UnixMilli())" }
		} else {
			rv.encFor = func(a string) string { return "runtime.EncodeEpochMS(" + a + ")" }
			rv.predFor = func(a string) string { return "runtime.PredEpochMS(" + a + ")" }
		}
	case "ulid":
		rv.EncHasErr = true
		rv.encFor = func(a string) string { return "runtime.EncodeULID(" + a + ")" }
		rv.predFor = func(a string) string { return "runtime.PredULID(" + a + ")" }
	case "hex":
		rv.encFor = func(a string) string { return "runtime.EncodeHex(" + a + "[:])" }
		rv.predFor = func(a string) string { return "runtime.PredHex(runtime.EncodeHex(" + a + "[:]))" }
	default:
		w := keytmpl.PadWidth(cut.Next.Encoder)
		if w == 0 {
			return nil, nil
		}
		ws := strconv.Itoa(w)
		rv.EncHasErr = true
		if strings.HasPrefix(f.GoType, "uint") {
			rv.encFor = func(a string) string { return "runtime.EncodePadUint(uint64(" + a + "), " + ws + ")" }
			rv.predFor = func(a string) string { return "runtime.PredPadUint(uint64(" + a + "), " + ws + ")" }
		} else {
			rv.encFor = func(a string) string { return "runtime.EncodePad(" + a + ", " + ws + ")" }
			rv.predFor = func(a string) string { return "runtime.PredPad(" + a + ", " + ws + ")" }
		}
	}
	return rv, nil
}

// valueExpr renders the Go expression assembling a condition value from
// its prefix segments, emitting encoding steps for placeholders. It
// returns the full expression, the quoted static part before the final
// placeholder (for predecessor assembly), and the final step variable.
//
// ensureDelim appends the delimiter regardless of whether the marker wrote
// one. Every begins prefix that stops short of the full sk template must
// end at a delimiter: "ORDER" would otherwise both fail to prefix-match
// real keys ("ORDER#...") in range bounds and over-match sibling literals
// ("ORDERX#..."). AlignPrefix validates the segment boundary; this makes
// the rendered string honor it.
func valueExpr(e *schema.Entity, p *keytmpl.Prefix, ensureDelim bool, steps *[]encStep) (full, staticPart, lastVar string, err error) {
	var pieces []string
	lit := ""
	for i, seg := range p.Segments {
		if i > 0 {
			lit += keytmpl.Delimiter
		}
		if seg.Kind == keytmpl.SegLiteral {
			lit += seg.Literal
			continue
		}
		f, ok := e.Field(seg.Field)
		if !ok {
			return "", "", "", fmt.Errorf("%s: condition %q references unknown field {%s}", e.Pos, p.Raw, seg.Field)
		}
		expr, hasErr, eerr := encodeExpr(seg.Encoder, f.GoType, lowerCamel(f.Name))
		if eerr != nil {
			return "", "", "", fmt.Errorf("%s: condition %q placeholder {%s}: %w", e.Pos, p.Raw, seg.Field, eerr)
		}
		if lit != "" {
			pieces = append(pieces, strconv.Quote(lit))
			lit = ""
		}
		v := "cs" + strconv.Itoa(len(*steps))
		*steps = append(*steps, encStep{Var: v, Expr: expr, HasErr: hasErr, Desc: "sk condition " + seg.String()})
		pieces = append(pieces, v)
		lastVar = v
	}
	if p.TrailingDelim || ensureDelim {
		lit += keytmpl.Delimiter
	}
	if lit != "" {
		pieces = append(pieces, strconv.Quote(lit))
	}
	if len(pieces) == 0 {
		return `""`, `""`, "", nil
	}
	full = strings.Join(pieces, " + ")
	if lastVar != "" && len(pieces) > 1 && pieces[len(pieces)-1] == lastVar {
		staticPart = strings.Join(pieces[:len(pieces)-1], " + ")
	}
	return full, staticPart, lastVar, nil
}

func buildPatternViews(t *schema.Table, e *schema.Entity, ev *entityView) error {
	for _, p := range e.Patterns {
		pv, err := buildPatternView(t, e, ev, p)
		if err != nil {
			return err
		}
		ev.Patterns = append(ev.Patterns, pv)
	}
	return nil
}

func buildPatternView(t *schema.Table, e *schema.Entity, ev *entityView, p *schema.Pattern) (*patternView, error) {
	key, ok := e.KeyFor(p.Index)
	if !ok {
		return nil, fmt.Errorf("%s: pattern %s references undeclared index %q", p.Pos, p.Name, p.Index)
	}
	pv := &patternView{
		Name:          p.Name,
		QueryType:     p.Name + "Query",
		Entity:        e.Name,
		Client:        ev.Client,
		PKAttr:        schema.PKAttrFor(p.Index),
		HasConsistent: p.Index == "main",
	}
	if p.Index != "main" {
		pv.Index = p.Index
	}
	if key.SK != nil {
		pv.SKAttr = schema.SKAttrFor(p.Index)
	}

	// Constructor params: the pattern's pk placeholders (structurally equal
	// to the entity's index pk template, so the entity key func is reused).
	pkFunc, err := buildKeyFunc(e, schema.PKAttrFor(p.Index), key.PK, ev.ItemVar)
	if err != nil {
		return nil, err
	}
	pv.PKFunc = pkFunc.Name
	pv.PKArgs = pkFunc.FromParam
	pv.Params = append(pv.Params, pkFunc.Params...)

	if key.SK == nil {
		pv.Doc = docFor(p, key, "")
		return pv, nil
	}

	scopeCut := keytmpl.LeadingLiteralCut(key.SK)
	scope := keytmpl.PrefixString(key.SK, scopeCut.Consumed)
	pv.ScopeQ = strconv.Quote(scope)

	types := func(field string) (string, bool) {
		f, ok := e.Field(field)
		return f.GoType, ok
	}

	var cut *keytmpl.Cut
	switch p.SKCond {
	case schema.SKNone, schema.SKBetween, schema.SKGt, schema.SKGte, schema.SKLt, schema.SKLte:
		if p.SKValue == nil {
			// Bare markers and condition-less patterns scope to the leading
			// literal prefix and refine through generated range methods.
			cut = scopeCut
			if scope != "" {
				pv.CondKind = "implicit"
				pv.ValExpr = strconv.Quote(scope)
			}
			break
		}
		fallthrough
	default:
		var err error
		cut, err = keytmpl.AlignPrefix(key.SK, p.SKValue, types)
		if err != nil {
			return nil, fmt.Errorf("%s: pattern %s: %w", p.Pos, p.Name, err)
		}
		// Only begins prefixes are prefix-matched strings that must land on
		// a delimiter; range operands (eq/gt/gte/lt/lte) are compared as
		// encoded values and must NOT gain one.
		ensureDelim := p.SKCond == schema.SKBegins && cut.Consumed < len(key.SK.Segments)
		full, staticPart, lastVar, err := valueExpr(e, p.SKValue, ensureDelim, &pv.CondSteps)
		if err != nil {
			return nil, err
		}
		pv.ValExpr = full
		pv.CondKind = string(p.SKCond)
		// Condition placeholders become constructor params, after pk's.
		pv.Params = dedupParams(append(pv.Params, prefixParams(e, p.SKValue)...))
		if p.SKCond == schema.SKLt {
			segs := p.SKValue.Segments
			if lastVar == "" || p.SKValue.TrailingDelim || segs[len(segs)-1].Kind != keytmpl.SegPlaceholder {
				return nil, fmt.Errorf("%s: pattern %s: sk.lt requires a value ending in a placeholder", p.Pos, p.Name)
			}
			last := segs[len(segs)-1]
			f, _ := e.Field(last.Field)
			call, err := predCallFor(last.Encoder, f.GoType, lowerCamel(f.Name))
			if err != nil {
				return nil, fmt.Errorf("%s: pattern %s: %w", p.Pos, p.Name, err)
			}
			pv.PredCall = call
			if staticPart == "" {
				pv.PredFull = "pred"
			} else {
				pv.PredFull = staticPart + " + pred"
			}
		}
	}

	// Range methods only when the static condition leaves a begins-style
	// prefix (none, implicit, or begins) with a fixed-width next placeholder.
	if pv.CondKind == "" || pv.CondKind == "implicit" || pv.CondKind == "begins" {
		rv, err := rangeFor(e, cut)
		if err != nil {
			return nil, err
		}
		pv.Range = rv
		pv.HasSKStore = rv != nil || pv.CondKind == "implicit" || pv.CondKind == "begins"
	}
	pv.HasCond = pv.CondKind != "" || pv.Range != nil
	pv.Doc = docFor(p, key, scope)
	return pv, nil
}

// predCallFor renders the predecessor call for a valued lt condition's
// final placeholder.
func predCallFor(encoder, goType, arg string) (string, error) {
	switch encoder {
	case "rfc3339":
		return "runtime.PredRFC3339(" + arg + ")", nil
	case "epoch":
		if goType == "time.Time" {
			return "runtime.PredEpoch(" + arg + ".Unix())", nil
		}
		return "runtime.PredEpoch(" + arg + ")", nil
	case "epochms":
		if goType == "time.Time" {
			return "runtime.PredEpochMS(" + arg + ".UnixMilli())", nil
		}
		return "runtime.PredEpochMS(" + arg + ")", nil
	case "ulid":
		return "runtime.PredULID(" + arg + ")", nil
	case "hex":
		return "runtime.PredHex(runtime.EncodeHex(" + arg + "[:]))", nil
	default:
		if w := keytmpl.PadWidth(encoder); w > 0 {
			ws := strconv.Itoa(w)
			if strings.HasPrefix(goType, "uint") {
				return "runtime.PredPadUint(uint64(" + arg + "), " + ws + ")", nil
			}
			return "runtime.PredPad(" + arg + ", " + ws + ")", nil
		}
		return "", fmt.Errorf("sk.lt is not supported on variable-width encoder %q", displayEncoder(encoder))
	}
}

func prefixParams(e *schema.Entity, p *keytmpl.Prefix) []param {
	var out []param
	for _, seg := range p.Placeholders() {
		f, ok := e.Field(seg.Field)
		if !ok {
			continue
		}
		out = append(out, param{Name: lowerCamel(f.Name), Field: f.Name, Type: f.GoType})
	}
	return out
}

func docFor(p *schema.Pattern, key schema.KeySpec, scope string) string {
	idx := "the main index"
	if p.Index != "main" {
		idx = "GSI " + p.Index
	}
	cond := fmt.Sprintf("pk = %q", key.PK.Raw)
	switch {
	case p.SKValue != nil:
		cond += fmt.Sprintf(", sk %s %q", p.SKCond, p.SKValue.Raw)
	case scope != "":
		cond += fmt.Sprintf(", sk begins_with %q", scope)
	}
	return fmt.Sprintf("%s on %s", cond, idx)
}

// batchView drives BatchGet/BatchPut generation for one entity.
type batchView struct {
	Entity string
	Plural string
	PKFunc string
	PKKey  string // k.TenantID, ...
	SKFunc string
	SKKey  string
	HasSK  bool
	HasVer bool
}

func buildBatchView(ev *entityView) batchView {
	bv := batchView{
		Entity: ev.Entity,
		Plural: plural(ev.Entity),
		PKFunc: ev.PKFunc.Name,
		PKKey:  itemArgList(ev.PKFunc.Params, "k"),
		HasSK:  ev.HasSK,
		HasVer: ev.HasVersion,
	}
	if ev.HasSK {
		bv.SKFunc = ev.SKFunc.Name
		bv.SKKey = itemArgList(ev.SKFunc.Params, "k")
	}
	return bv
}

func plural(s string) string {
	switch {
	case strings.HasSuffix(s, "s"), strings.HasSuffix(s, "x"), strings.HasSuffix(s, "z"),
		strings.HasSuffix(s, "ch"), strings.HasSuffix(s, "sh"):
		return s + "es"
	case strings.HasSuffix(s, "y") && len(s) > 1 && !strings.ContainsRune("aeiou", rune(s[len(s)-2])):
		return s[:len(s)-1] + "ies"
	default:
		return s + "s"
	}
}

// collMember is one entity dispatched inside a collection.
type collMember struct {
	Entity    string
	Plural    string
	ETAttr    string
	Type      string
	Unmarshal string
}

// partitionView drives one generated partition query + collection.
type partitionView struct {
	Name       string // TenantPartition
	QueryType  string // TenantPartitionQuery
	Collection string // TenantCollection
	Client     string
	Params     []param
	PKFunc     string
	PKArgs     string
	PKRaw      string
	Members    []collMember
}

// buildPartitionViews groups entities sharing a structurally equal main pk
// template and emits a typed partition query per multi-entity group.
func buildPartitionViews(t *schema.Table) ([]*partitionView, error) {
	type group struct {
		skeleton string
		ents     []*schema.Entity
	}
	byKey := map[string]*group{}
	var order []string
	for _, e := range t.Entities {
		var parts []string
		for _, seg := range e.Key.PK.Segments {
			if seg.Kind == keytmpl.SegLiteral {
				parts = append(parts, "lit:"+seg.Literal)
			} else {
				parts = append(parts, "ph:"+seg.Encoder)
			}
		}
		key := strings.Join(parts, "|")
		g, ok := byKey[key]
		if !ok {
			g = &group{skeleton: key}
			byKey[key] = g
			order = append(order, key)
		}
		g.ents = append(g.ents, e)
	}
	sort.Strings(order)

	used := map[string]bool{}
	var out []*partitionView
	for _, key := range order {
		g := byKey[key]
		if len(g.ents) < 2 {
			continue
		}
		first := g.ents[0]
		base := exported(strings.ToLower(t.Name))
		if seg := first.Key.PK.Segments[0]; seg.Kind == keytmpl.SegLiteral {
			base = exported(strings.ToLower(seg.Literal))
		}
		name := base + "Partition"
		for i := 2; used[name]; i++ {
			name = base + strconv.Itoa(i) + "Partition"
		}
		used[name] = true

		pkFunc, err := buildKeyFunc(first, "pk", first.Key.PK, "v")
		if err != nil {
			return nil, err
		}
		pv := &partitionView{
			Name:       name,
			QueryType:  name + "Query",
			Collection: strings.TrimSuffix(name, "Partition") + "Collection",
			Client:     exported(t.Name) + "Client",
			Params:     pkFunc.Params,
			PKFunc:     pkFunc.Name,
			PKArgs:     pkFunc.FromParam,
			PKRaw:      first.Key.PK.Raw,
		}
		for _, e := range g.ents {
			pv.Members = append(pv.Members, collMember{
				Entity:    e.Name,
				Plural:    plural(e.Name),
				ETAttr:    e.ETAttr,
				Type:      e.Type,
				Unmarshal: "unmarshal" + e.Name,
			})
		}
		out = append(out, pv)
	}
	return out, nil
}
