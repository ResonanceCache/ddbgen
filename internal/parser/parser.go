// Package parser loads Go packages, finds structs annotated with //ddb:
// marker comments, and compiles them into the schema IR. Parsing is purely
// syntactic: no type checking is performed, and field types are recorded as
// source expressions for downstream semantic checks in internal/analyze.
package parser

import (
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/ResonanceCache/ddbgen/internal/keytmpl"
	"github.com/ResonanceCache/ddbgen/internal/schema"
)

var (
	identRe       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	patternNameRe = regexp.MustCompile(`^[A-Z][A-Za-z0-9_]*$`)
)

// Load parses the packages matched by the given patterns (like go vet:
// ./... etc.) and compiles all annotated structs into a schema.
func Load(patterns ...string) (*schema.Schema, error) {
	if len(patterns) == 0 {
		patterns = []string{"."}
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax,
		Fset: token.NewFileSet(),
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}
	var entities []*schema.Entity
	for _, pkg := range pkgs {
		for _, perr := range pkg.Errors {
			return nil, fmt.Errorf("loading %s: %s", pkg.PkgPath, perr)
		}
		for _, f := range pkg.Syntax {
			filename := cfg.Fset.Position(f.Pos()).Filename
			ents, err := entitiesFromFile(cfg.Fset, f, pkg.Name, filepath.Dir(filename))
			if err != nil {
				return nil, err
			}
			entities = append(entities, ents...)
		}
	}
	if len(entities) == 0 {
		return nil, fmt.Errorf("no //ddb: annotated structs found in %s", strings.Join(patterns, " "))
	}
	return schema.Compile(entities)
}

// ParseSource compiles a single Go source file, for tests and tooling.
func ParseSource(filename string, src []byte) (*schema.Schema, error) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, filename, src, goparser.ParseComments)
	if err != nil {
		return nil, err
	}
	entities, err := entitiesFromFile(fset, f, f.Name.Name, filepath.Dir(filename))
	if err != nil {
		return nil, err
	}
	if len(entities) == 0 {
		return nil, fmt.Errorf("%s: no //ddb: annotated structs found", filename)
	}
	return schema.Compile(entities)
}

func entitiesFromFile(fset *token.FileSet, f *ast.File, pkgName, dir string) ([]*schema.Entity, error) {
	var out []*schema.Entity
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			doc := ts.Doc
			if doc == nil && len(gd.Specs) == 1 {
				doc = gd.Doc
			}
			if doc == nil {
				continue
			}
			markers, err := collectMarkers(fset, doc)
			if err != nil {
				return nil, err
			}
			if len(markers) == 0 {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return nil, markers[0].errAt(0, "//ddb: markers must annotate a struct type; %s is not a struct", ts.Name.Name)
			}
			ent, err := buildEntity(ts.Name.Name, st, markers, pkgName, dir)
			if err != nil {
				return nil, err
			}
			out = append(out, ent)
		}
	}
	return out, nil
}

func collectMarkers(fset *token.FileSet, doc *ast.CommentGroup) ([]*marker, error) {
	var out []*marker
	for _, c := range doc.List {
		if !strings.HasPrefix(c.Text, markerPrefix) {
			continue
		}
		pos := fset.Position(c.Slash)
		m := &marker{text: c.Text, file: pos.Filename, line: pos.Line, col: pos.Column}
		if err := lexMarker(m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

var skCondKeys = map[string]schema.SKCondKind{
	"sk.eq":      schema.SKEq,
	"sk.begins":  schema.SKBegins,
	"sk.between": schema.SKBetween,
	"sk.gt":      schema.SKGt,
	"sk.gte":     schema.SKGte,
	"sk.lt":      schema.SKLt,
	"sk.lte":     schema.SKLte,
}

func buildEntity(name string, st *ast.StructType, markers []*marker, pkgName, dir string) (*schema.Entity, error) {
	ent := &schema.Entity{
		Name:      name,
		ETAttr:    schema.DefaultETAttr,
		Fields:    collectFields(st),
		GoPackage: pkgName,
		Dir:       dir,
	}
	var entityM, keyM *marker
	indexNames := map[string]*marker{}
	patternNames := map[string]*marker{}

	for _, m := range markers {
		switch m.directive {
		case "entity":
			if entityM != nil {
				return nil, m.errAt(0, "duplicate //ddb:entity marker (first at line %d)", entityM.line)
			}
			entityM = m
			if err := parseEntityMarker(m, ent); err != nil {
				return nil, err
			}
		case "key":
			if keyM != nil {
				return nil, m.errAt(0, "duplicate //ddb:key marker (first at line %d)", keyM.line)
			}
			keyM = m
			if err := parseKeyMarker(m, ent); err != nil {
				return nil, err
			}
		case "index":
			ix, nameArg, err := parseIndexMarker(m)
			if err != nil {
				return nil, err
			}
			if prev, dup := indexNames[ix.Name]; dup {
				return nil, m.errAt(nameArg.valOff, "duplicate //ddb:index name %q (first at line %d)", ix.Name, prev.line)
			}
			indexNames[ix.Name] = m
			ent.Indexes = append(ent.Indexes, ix)
		case "pattern":
			p, nameArg, err := parsePatternMarker(m)
			if err != nil {
				return nil, err
			}
			if prev, dup := patternNames[p.Name]; dup {
				return nil, m.errAt(nameArg.valOff, "duplicate //ddb:pattern name %q (first at line %d)", p.Name, prev.line)
			}
			patternNames[p.Name] = m
			ent.Patterns = append(ent.Patterns, p)
		default:
			return nil, m.errAt(len(markerPrefix), "unknown directive %q (want entity, key, index, or pattern)", m.directive)
		}
	}

	if entityM == nil {
		return nil, markers[0].errAt(0, "struct %s has //ddb: markers but no //ddb:entity marker", name)
	}
	if keyM == nil {
		return nil, entityM.errAt(0, "entity %s has no //ddb:key marker", name)
	}
	// Patterns may reference indexes declared on any line of the block, so
	// resolve references after the full block is read.
	for _, p := range ent.Patterns {
		if p.Index == "main" {
			continue
		}
		if _, ok := indexNames[p.Index]; !ok {
			m := patternNames[p.Name]
			return nil, m.errAt(argOff(m, "index"), "pattern %s references index %q, but entity %s declares no //ddb:index with that name", p.Name, p.Index, name)
		}
	}
	return ent, nil
}

func argOff(m *marker, key string) int {
	for _, a := range m.args {
		if a.key == key {
			return a.valOff
		}
	}
	return len(markerPrefix)
}

func parseEntityMarker(m *marker, ent *schema.Entity) error {
	set, err := m.argSet("table", "type", "version", "ttl", "et")
	if err != nil {
		return err
	}
	tbl, err := m.need(set, "table")
	if err != nil {
		return err
	}
	if !identRe.MatchString(tbl.value) {
		return m.errAt(tbl.valOff, "invalid table name %q (want an identifier)", tbl.value)
	}
	typ, err := m.need(set, "type")
	if err != nil {
		return err
	}
	if !identRe.MatchString(typ.value) {
		return m.errAt(typ.valOff, "invalid entity type %q (want an identifier)", typ.value)
	}
	ent.Table = tbl.value
	ent.Type = typ.value
	ent.Pos = schema.Pos{File: m.file, Line: m.line}
	if a, ok := set["version"]; ok {
		if _, err := m.need(set, "version"); err != nil {
			return err
		}
		ent.VersionField = a.value
	}
	if a, ok := set["ttl"]; ok {
		if _, err := m.need(set, "ttl"); err != nil {
			return err
		}
		ent.TTLField = a.value
	}
	if a, ok := set["et"]; ok {
		if _, err := m.need(set, "et"); err != nil {
			return err
		}
		ent.ETAttr = a.value
	}
	return nil
}

func parseKeyMarker(m *marker, ent *schema.Entity) error {
	set, err := m.argSet("pk", "sk")
	if err != nil {
		return err
	}
	key, err := parseKeySpec(m, set)
	if err != nil {
		return err
	}
	ent.Key = key
	ent.KeyPos = schema.Pos{File: m.file, Line: m.line}
	return nil
}

func parseKeySpec(m *marker, set map[string]arg) (schema.KeySpec, error) {
	var out schema.KeySpec
	pk, err := m.need(set, "pk")
	if err != nil {
		return out, err
	}
	out.PK, err = parseTemplateArg(m, pk)
	if err != nil {
		return out, err
	}
	if sk, ok := set["sk"]; ok {
		if _, err := m.need(set, "sk"); err != nil {
			return out, err
		}
		out.SK, err = parseTemplateArg(m, sk)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func parseTemplateArg(m *marker, a arg) (*keytmpl.Template, error) {
	t, err := keytmpl.Parse(a.value)
	if err != nil {
		return nil, templateErr(m, a, err)
	}
	return t, nil
}

func templateErr(m *marker, a arg, err error) error {
	var perr *keytmpl.ParseError
	if pe, ok := err.(*keytmpl.ParseError); ok {
		perr = pe
	} else {
		perr = &keytmpl.ParseError{Msg: err.Error()}
	}
	return m.errAt(a.valOff+perr.Offset, "%s", perr.Msg)
}

func parseIndexMarker(m *marker) (schema.EntityIndex, arg, error) {
	var ix schema.EntityIndex
	set, err := m.argSet("name", "pk", "sk", "projection")
	if err != nil {
		return ix, arg{}, err
	}
	nameArg, err := m.need(set, "name")
	if err != nil {
		return ix, arg{}, err
	}
	if !identRe.MatchString(nameArg.value) {
		return ix, arg{}, m.errAt(nameArg.valOff, "invalid index name %q (want an identifier)", nameArg.value)
	}
	if nameArg.value == "main" {
		return ix, arg{}, m.errAt(nameArg.valOff, `index name "main" is reserved for the table's primary key`)
	}
	ix.Name = nameArg.value
	ix.Key, err = parseKeySpec(m, set)
	if err != nil {
		return ix, arg{}, err
	}
	ix.Projection = "all"
	if a, ok := set["projection"]; ok {
		if _, err := m.need(set, "projection"); err != nil {
			return ix, arg{}, err
		}
		if a.value != "all" && a.value != "keys_only" {
			return ix, arg{}, m.errAt(a.valOff, "invalid projection %q (want all or keys_only)", a.value)
		}
		ix.Projection = a.value
	}
	ix.Pos = schema.Pos{File: m.file, Line: m.line}
	return ix, nameArg, nil
}

func parsePatternMarker(m *marker) (*schema.Pattern, arg, error) {
	set, err := m.argSet("name", "index", "pk",
		"sk.eq", "sk.begins", "sk.between", "sk.gt", "sk.gte", "sk.lt", "sk.lte")
	if err != nil {
		return nil, arg{}, err
	}
	nameArg, err := m.need(set, "name")
	if err != nil {
		return nil, arg{}, err
	}
	if !patternNameRe.MatchString(nameArg.value) {
		return nil, arg{}, m.errAt(nameArg.valOff, "invalid pattern name %q (want an exported Go identifier, e.g. OrdersByTenant)", nameArg.value)
	}
	idx, err := m.need(set, "index")
	if err != nil {
		return nil, arg{}, err
	}
	if idx.value != "main" && !identRe.MatchString(idx.value) {
		return nil, arg{}, m.errAt(idx.valOff, "invalid index reference %q (want main or a GSI name)", idx.value)
	}
	pkArg, err := m.need(set, "pk")
	if err != nil {
		return nil, arg{}, err
	}
	pk, err := parseTemplateArg(m, pkArg)
	if err != nil {
		return nil, arg{}, err
	}
	p := &schema.Pattern{
		Name:  nameArg.value,
		Index: idx.value,
		PK:    pk,
		Pos:   schema.Pos{File: m.file, Line: m.line},
	}
	for key, kind := range skCondKeys {
		a, ok := set[key]
		if !ok {
			continue
		}
		if p.SKCond != schema.SKNone {
			return nil, arg{}, m.errAt(a.keyOff, "conflicting sort-key conditions: a pattern takes at most one sk.* condition")
		}
		p.SKCond = kind
		switch kind {
		case schema.SKBetween:
			if a.hasValue {
				return nil, arg{}, m.errAt(a.valOff, "sk.between takes no value; both bounds are supplied through the generated Between method")
			}
		case schema.SKEq, schema.SKBegins:
			if !a.hasValue {
				return nil, arg{}, m.errAt(a.keyOff, "%s requires a value", key)
			}
		}
		if a.hasValue {
			v, err := keytmpl.ParsePrefix(a.value)
			if err != nil {
				return nil, arg{}, templateErr(m, a, err)
			}
			p.SKValue = v
		}
	}
	return p, nameArg, nil
}

func collectFields(st *ast.StructType) []schema.Field {
	var out []schema.Field
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			continue // embedded fields are not addressable by markers
		}
		goType := types.ExprString(f.Type)
		attr, omitEmpty, skip := parseFieldTag(f.Tag)
		if skip {
			continue
		}
		for _, n := range f.Names {
			if !n.IsExported() {
				continue
			}
			name := attr
			if name == "" {
				name = n.Name
			}
			out = append(out, schema.Field{
				Name:      n.Name,
				GoType:    goType,
				Attr:      name,
				OmitEmpty: omitEmpty,
			})
		}
	}
	return out
}

func parseFieldTag(tag *ast.BasicLit) (attr string, omitEmpty, skip bool) {
	if tag == nil {
		return "", false, false
	}
	val := reflect.StructTag(strings.Trim(tag.Value, "`")).Get("dynamodbav")
	if val == "" {
		return "", false, false
	}
	parts := strings.Split(val, ",")
	if parts[0] == "-" {
		return "", false, true
	}
	for _, p := range parts[1:] {
		if p == "omitempty" {
			omitEmpty = true
		}
	}
	return parts[0], omitEmpty, false
}
