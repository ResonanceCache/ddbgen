package codegen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ResonanceCache/ddbgen/internal/keytmpl"
	"github.com/ResonanceCache/ddbgen/internal/schema"
)

// param is one generated function parameter sourced from a struct field.
type param struct {
	Name  string // tenantID
	Field string // TenantID
	Type  string // string, time.Time, ...
}

func paramList(ps []param) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.Name + " " + p.Type
	}
	return strings.Join(parts, ", ")
}

func argList(ps []param) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.Name
	}
	return strings.Join(parts, ", ")
}

func itemArgList(ps []param, itemVar string) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = itemVar + "." + p.Field
	}
	return strings.Join(parts, ", ")
}

// encStep is one placeholder encoding statement inside a key function.
type encStep struct {
	Var    string // s0
	Expr   string // runtime.EncodeRFC3339(createdAt)
	HasErr bool
	Desc   string // pk {CreatedAt:rfc3339}
}

// keyFunc is a generated key-encoder function view.
type keyFunc struct {
	Name      string // orderSK
	Desc      string // sort key ("ORDER#{CreatedAt:rfc3339}#{OrderID}")
	Attr      string // sk
	Params    []param
	Steps     []encStep
	Return    string // `"ORDER#" + s0 + "#" + s1`
	FromItem  string // v.CreatedAt, v.OrderID (args for marshal call sites)
	FromParam string // createdAt, orderID
}

// entityView drives the per-entity file template.
type entityView struct {
	Package    string
	Entity     string // Order
	ItemVar    string // o
	Client     string // AppClient
	ItemIface  string // AppItem
	TypeString string // order
	ETAttr     string // _et

	KeyFuncs []keyFunc
	PKFunc   keyFunc
	SKFunc   keyFunc
	HasSK    bool

	KeyParams []param // dedup pk+sk placeholders, template order

	HasVersion   bool
	VersionField string // Ver
	VersionAttr  string // v
	VersionConv  string // "" or "int64(...)" conversion needed

	HasTTL bool

	Updates []updateMethod
}

// updateMethod is one generated method on the update builder.
type updateMethod struct {
	Kind     string // set, add, remove
	Method   string // SetStatus
	Field    string // Status
	Param    string // status
	Type     string // string
	Attr     string // status
	NumConv  string // FormatInt/FormatUint conversion expr for add
	GSISyncs []gsiSync
}

// gsiSync recomputes a synthesized GSI key attribute inside a setter.
type gsiSync struct {
	Attr     string // gsi1pk
	FuncName string // orderGSI1PK
	Index    string // GSI1
}

// tableView drives the per-table file template.
type tableView struct {
	Package  string
	Table    string
	Client   string // AppClient
	Iface    string // AppItem
	Entities []string
}

func buildTableView(t *schema.Table) tableView {
	tv := tableView{
		Package: t.GoPackage,
		Table:   t.Name,
		Client:  exported(t.Name) + "Client",
		Iface:   exported(t.Name) + "Item",
	}
	for _, e := range t.Entities {
		tv.Entities = append(tv.Entities, e.Name)
	}
	return tv
}

func buildEntityView(t *schema.Table, e *schema.Entity) (*entityView, error) {
	ev := &entityView{
		Package:    t.GoPackage,
		Entity:     e.Name,
		ItemVar:    itemVar(e.Name),
		Client:     exported(t.Name) + "Client",
		ItemIface:  exported(t.Name) + "Item",
		TypeString: e.Type,
		ETAttr:     e.ETAttr,
		HasSK:      e.Key.SK != nil,
		HasTTL:     e.TTLField != "",
	}

	pkFunc, err := buildKeyFunc(e, "pk", e.Key.PK, ev.ItemVar)
	if err != nil {
		return nil, err
	}
	ev.PKFunc = pkFunc
	ev.KeyFuncs = append(ev.KeyFuncs, pkFunc)
	if e.Key.SK != nil {
		skFunc, err := buildKeyFunc(e, "sk", e.Key.SK, ev.ItemVar)
		if err != nil {
			return nil, err
		}
		ev.SKFunc = skFunc
		ev.KeyFuncs = append(ev.KeyFuncs, skFunc)
	}
	for _, ix := range e.Indexes {
		f, err := buildKeyFunc(e, schema.PKAttrFor(ix.Name), ix.Key.PK, ev.ItemVar)
		if err != nil {
			return nil, err
		}
		ev.KeyFuncs = append(ev.KeyFuncs, f)
		if ix.Key.SK != nil {
			f, err := buildKeyFunc(e, schema.SKAttrFor(ix.Name), ix.Key.SK, ev.ItemVar)
			if err != nil {
				return nil, err
			}
			ev.KeyFuncs = append(ev.KeyFuncs, f)
		}
	}

	ev.KeyParams = dedupParams(append(append([]param{}, pkFunc.Params...), ev.SKFunc.Params...))

	if e.VersionField != "" {
		f, ok := e.Field(e.VersionField)
		if !ok {
			return nil, fmt.Errorf("entity %s: version field %s not found", e.Name, e.VersionField)
		}
		ev.HasVersion = true
		ev.VersionField = f.Name
		ev.VersionAttr = f.Attr
		if f.GoType != "int64" {
			ev.VersionConv = "int64"
		}
	}

	ups, err := buildUpdateMethods(e)
	if err != nil {
		return nil, err
	}
	ev.Updates = ups
	return ev, nil
}

func itemVar(entity string) string {
	v := strings.ToLower(entity[:1])
	if reservedIdents[v] || v == "s" {
		return "it"
	}
	return v
}

func dedupParams(ps []param) []param {
	seen := map[string]bool{}
	out := ps[:0]
	for _, p := range ps {
		if seen[p.Field] {
			continue
		}
		seen[p.Field] = true
		out = append(out, p)
	}
	return out
}

func buildKeyFunc(e *schema.Entity, attr string, tmpl *keytmpl.Template, itemVar string) (keyFunc, error) {
	kf := keyFunc{
		Name: lowerCamel(e.Name) + strings.ToUpper(attr),
		Desc: attr + " (\"" + tmpl.Raw + "\")",
		Attr: attr,
	}
	var pieces []string // alternating quoted literals and step vars
	lit := ""
	seen := map[string]int{} // field -> step index
	for i, seg := range tmpl.Segments {
		if i > 0 {
			lit += keytmpl.Delimiter
		}
		if seg.Kind == keytmpl.SegLiteral {
			lit += seg.Literal
			continue
		}
		f, ok := e.Field(seg.Field)
		if !ok {
			return kf, fmt.Errorf("%s: template %q references unknown field {%s}", e.Pos, tmpl.Raw, seg.Field)
		}
		var stepVar string
		if idx, dup := seen[seg.Field]; dup {
			stepVar = kf.Steps[idx].Var
		} else {
			p := param{Name: lowerCamel(f.Name), Field: f.Name, Type: f.GoType}
			kf.Params = append(kf.Params, p)
			expr, hasErr, err := encodeExpr(seg.Encoder, f.GoType, p.Name)
			if err != nil {
				return kf, fmt.Errorf("%s: template %q placeholder {%s}: %w", e.Pos, tmpl.Raw, seg.Field, err)
			}
			stepVar = "s" + strconv.Itoa(len(kf.Steps))
			seen[seg.Field] = len(kf.Steps)
			kf.Steps = append(kf.Steps, encStep{
				Var:    stepVar,
				Expr:   expr,
				HasErr: hasErr,
				Desc:   attr + " " + seg.String(),
			})
		}
		if lit != "" {
			pieces = append(pieces, strconv.Quote(lit))
			lit = ""
		}
		pieces = append(pieces, stepVar)
	}
	if lit != "" {
		pieces = append(pieces, strconv.Quote(lit))
	}
	kf.Return = strings.Join(pieces, " + ")
	kf.FromItem = itemArgList(kf.Params, itemVar)
	kf.FromParam = argList(kf.Params)
	return kf, nil
}

// encodeExpr returns the runtime call encoding one placeholder.
func encodeExpr(encoder, goType, arg string) (expr string, hasErr bool, err error) {
	fail := func() (string, bool, error) {
		return "", false, fmt.Errorf("encoder %q does not support Go type %s", displayEncoder(encoder), goType)
	}
	switch encoder {
	case "":
		if goType != "string" {
			return fail()
		}
		return "runtime.CheckSegment(" + arg + ")", true, nil
	case "rfc3339":
		if goType != "time.Time" {
			return fail()
		}
		return "runtime.EncodeRFC3339(" + arg + ")", true, nil
	case "epoch":
		switch goType {
		case "time.Time":
			return "runtime.EncodeEpochTime(" + arg + ")", true, nil
		case "int64":
			return "runtime.EncodeEpoch(" + arg + ")", true, nil
		}
		return fail()
	case "epochms":
		switch goType {
		case "time.Time":
			return "runtime.EncodeEpochMSTime(" + arg + ")", true, nil
		case "int64":
			return "runtime.EncodeEpochMS(" + arg + ")", true, nil
		}
		return fail()
	case "upper":
		if goType != "string" {
			return fail()
		}
		return "runtime.CheckSegment(strings.ToUpper(" + arg + "))", true, nil
	case "lower":
		if goType != "string" {
			return fail()
		}
		return "runtime.CheckSegment(strings.ToLower(" + arg + "))", true, nil
	case "hex":
		if goType == "[]byte" {
			return "runtime.EncodeHex(" + arg + ")", false, nil
		}
		if _, ok := byteArray(goType); ok {
			return "runtime.EncodeHex(" + arg + "[:])", false, nil
		}
		return fail()
	case "ulid":
		if goType != "string" {
			return fail()
		}
		return "runtime.EncodeULID(" + arg + ")", true, nil
	case "urlenc":
		if goType != "string" {
			return fail()
		}
		return "runtime.EncodeURL(" + arg + ")", false, nil
	}
	if w := keytmpl.PadWidth(encoder); w > 0 {
		ws := strconv.Itoa(w)
		switch goType {
		case "int64":
			return "runtime.EncodePad(" + arg + ", " + ws + ")", true, nil
		case "uint", "uint8", "uint16", "uint32", "uint64":
			return "runtime.EncodePadUint(uint64(" + arg + "), " + ws + ")", true, nil
		}
		return fail()
	}
	return "", false, fmt.Errorf("unknown encoder %q", encoder)
}

func displayEncoder(enc string) string {
	if enc == "" {
		return "(none)"
	}
	return enc
}

func byteArray(goType string) (int, bool) {
	if !strings.HasPrefix(goType, "[") || !strings.HasSuffix(goType, "]byte") {
		return 0, false
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(goType, "["), "]byte")
	n, err := strconv.Atoi(inner)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

var intTypes = map[string]string{
	"int": "FormatInt", "int8": "FormatInt", "int16": "FormatInt",
	"int32": "FormatInt", "int64": "FormatInt",
	"uint": "FormatUint", "uint8": "FormatUint", "uint16": "FormatUint",
	"uint32": "FormatUint", "uint64": "FormatUint",
}

func buildUpdateMethods(e *schema.Entity) ([]updateMethod, error) {
	keyFields := map[string]bool{}
	for _, seg := range e.Key.PK.Placeholders() {
		keyFields[seg.Field] = true
	}
	if e.Key.SK != nil {
		for _, seg := range e.Key.SK.Placeholders() {
			keyFields[seg.Field] = true
		}
	}

	// For every field, find the GSI key templates it feeds and whether it is
	// their sole placeholder (only then can a setter recompute the key).
	type tmplRef struct {
		attr string
		ix   string
		sole bool
	}
	fieldGSIs := map[string][]tmplRef{}
	for _, ix := range e.Indexes {
		add := func(attr string, tm *keytmpl.Template) {
			phs := tm.Placeholders()
			for _, seg := range phs {
				fieldGSIs[seg.Field] = append(fieldGSIs[seg.Field], tmplRef{attr: attr, ix: ix.Name, sole: len(phs) == 1})
			}
		}
		add(schema.PKAttrFor(ix.Name), ix.Key.PK)
		if ix.Key.SK != nil {
			add(schema.SKAttrFor(ix.Name), ix.Key.SK)
		}
	}

	var out []updateMethod
	for _, f := range e.Fields {
		if keyFields[f.Name] || f.Name == e.VersionField {
			continue
		}
		refs := fieldGSIs[f.Name]
		blocked := false
		for _, r := range refs {
			if !r.sole {
				blocked = true // multi-field GSI template: a lone setter cannot recompute the key
			}
		}
		if blocked {
			continue
		}
		set := updateMethod{
			Kind:   "set",
			Method: "Set" + f.Name,
			Field:  f.Name,
			Param:  lowerCamel(f.Name),
			Type:   f.GoType,
			Attr:   f.Attr,
		}
		for _, r := range refs {
			set.GSISyncs = append(set.GSISyncs, gsiSync{
				Attr:     r.attr,
				FuncName: lowerCamel(e.Name) + strings.ToUpper(r.attr),
				Index:    r.ix,
			})
		}
		out = append(out, set)

		if format, isInt := intTypes[f.GoType]; isInt && len(refs) == 0 {
			var conv string
			switch {
			case f.GoType == "int64" || f.GoType == "uint64":
				conv = "delta"
			case format == "FormatUint":
				conv = "uint64(delta)"
			default:
				conv = "int64(delta)"
			}
			out = append(out, updateMethod{
				Kind:    "add",
				Method:  "Add" + f.Name,
				Field:   f.Name,
				Param:   "delta",
				Type:    f.GoType,
				Attr:    f.Attr,
				NumConv: "strconv." + format + "(" + conv + ", 10)",
			})
		}
		if len(refs) == 0 {
			out = append(out, updateMethod{
				Kind:   "remove",
				Method: "Remove" + f.Name,
				Field:  f.Name,
				Attr:   f.Attr,
			})
		}
	}
	return out, nil
}
