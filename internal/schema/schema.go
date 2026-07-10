// Package schema defines the compiled intermediate representation (IR) of
// a ddbgen single-table design: tables, entities, keys, indexes, and access
// patterns. The parser produces IR; analysis, code generation, and emitters
// consume only IR. The IR is JSON-serializable and forms the snapshot format.
package schema

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ResonanceCache/ddbgen/internal/keytmpl"
)

// Pos locates a marker in source for diagnostics. Excluded from snapshots.
type Pos struct {
	File string `json:"-"`
	Line int    `json:"-"`
}

func (p Pos) String() string { return fmt.Sprintf("%s:%d", p.File, p.Line) }

// Schema is the root IR node.
type Schema struct {
	Tables []*Table `json:"tables"`
}

// Table groups the entities that share one physical DynamoDB table.
type Table struct {
	Name     string    `json:"name"`
	Entities []*Entity `json:"entities"`
	// Indexes are the GSI definitions merged across entities, sorted by name.
	Indexes []*Index `json:"indexes,omitempty"`

	// GoPackage is the package name generated code belongs to.
	GoPackage string `json:"go_package"`
	// Dir is the directory generated files are written to.
	Dir string `json:"-"`
}

// Index is a table-level GSI definition.
type Index struct {
	Name       string `json:"name"`
	PKAttr     string `json:"pk_attr"`
	SKAttr     string `json:"sk_attr,omitempty"`
	Projection string `json:"projection"` // "all" or "keys_only"
}

// KeySpec is a pk template plus optional sk template.
type KeySpec struct {
	PK *keytmpl.Template `json:"pk"`
	SK *keytmpl.Template `json:"sk,omitempty"`
}

// EntityIndex is one entity's key templates for a named GSI.
type EntityIndex struct {
	Name       string  `json:"name"`
	Key        KeySpec `json:"key"`
	Projection string  `json:"projection"`
	Pos        Pos     `json:"-"`
}

// SKCondKind enumerates pattern sort-key condition kinds.
type SKCondKind string

const (
	SKNone    SKCondKind = ""
	SKEq      SKCondKind = "eq"
	SKBegins  SKCondKind = "begins"
	SKBetween SKCondKind = "between"
	SKGt      SKCondKind = "gt"
	SKGte     SKCondKind = "gte"
	SKLt      SKCondKind = "lt"
	SKLte     SKCondKind = "lte"
)

// Pattern is a declared access pattern compiled from a //ddb:pattern marker.
type Pattern struct {
	Name  string            `json:"name"`
	Index string            `json:"index"` // "main" or a GSI name
	PK    *keytmpl.Template `json:"pk"`
	// SKCond is the static sort-key condition kind, if any.
	SKCond SKCondKind `json:"sk_cond,omitempty"`
	// SKValue is the condition operand for valued conditions (eq, begins,
	// gt, gte, lt, lte). Bare between/gt/gte/lt/lte markers have none: the
	// bound is supplied at call time through generated boundary methods.
	SKValue *keytmpl.Prefix `json:"sk_value,omitempty"`
	Pos     Pos             `json:"-"`
}

// Field is one exported struct field visible to marshaling.
type Field struct {
	Name      string `json:"name"`
	GoType    string `json:"go_type"`
	Attr      string `json:"attr"`
	OmitEmpty bool   `json:"omit_empty,omitempty"`
}

// Entity is one annotated struct compiled to IR.
type Entity struct {
	Name         string        `json:"name"` // Go struct name
	Table        string        `json:"table"`
	Type         string        `json:"type"`    // value stored in the entity-type attribute
	ETAttr       string        `json:"et_attr"` // entity-type attribute name, default "_et"
	VersionField string        `json:"version_field,omitempty"`
	TTLField     string        `json:"ttl_field,omitempty"`
	Key          KeySpec       `json:"key"`
	Indexes      []EntityIndex `json:"indexes,omitempty"`
	Patterns     []*Pattern    `json:"patterns,omitempty"`
	Fields       []Field       `json:"fields"`

	GoPackage string `json:"go_package"`
	Dir       string `json:"-"`
	Pos       Pos    `json:"-"`
	KeyPos    Pos    `json:"-"`
}

// DefaultETAttr is the entity-type attribute name unless overridden by et=.
const DefaultETAttr = "_et"

// Field returns the field named name, if present.
func (e *Entity) Field(name string) (Field, bool) {
	for _, f := range e.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// IndexNamed returns the entity's key templates for the named GSI.
func (e *Entity) IndexNamed(name string) (EntityIndex, bool) {
	for _, ix := range e.Indexes {
		if ix.Name == name {
			return ix, true
		}
	}
	return EntityIndex{}, false
}

// KeyFor returns the entity's key templates on "main" or a GSI name.
func (e *Entity) KeyFor(index string) (KeySpec, bool) {
	if index == "main" {
		return e.Key, true
	}
	ix, ok := e.IndexNamed(index)
	if !ok {
		return KeySpec{}, false
	}
	return ix.Key, true
}

// PKAttrFor returns the physical partition-key attribute for an index name.
func PKAttrFor(index string) string {
	if index == "main" {
		return "pk"
	}
	return strings.ToLower(index) + "pk"
}

// SKAttrFor returns the physical sort-key attribute for an index name.
func SKAttrFor(index string) string {
	if index == "main" {
		return "sk"
	}
	return strings.ToLower(index) + "sk"
}

// Compile groups parsed entities into tables, merges GSI definitions, and
// applies deterministic ordering. It rejects structural conflicts that make
// a single physical table definition impossible.
func Compile(entities []*Entity) (*Schema, error) {
	byTable := map[string][]*Entity{}
	for _, e := range entities {
		byTable[e.Table] = append(byTable[e.Table], e)
	}
	names := make([]string, 0, len(byTable))
	for n := range byTable {
		names = append(names, n)
	}
	sort.Strings(names)

	s := &Schema{}
	for _, name := range names {
		ents := byTable[name]
		sort.Slice(ents, func(i, j int) bool { return ents[i].Name < ents[j].Name })
		t := &Table{Name: name, Entities: ents, GoPackage: ents[0].GoPackage, Dir: ents[0].Dir}
		for _, e := range ents {
			if e.GoPackage != t.GoPackage {
				return nil, fmt.Errorf("%s: table %q spans Go packages %q and %q; all entities of one table must live in one package",
					e.Pos, name, t.GoPackage, e.GoPackage)
			}
			if (e.Key.SK != nil) != (ents[0].Key.SK != nil) {
				return nil, fmt.Errorf("%s: entity %s disagrees with entity %s (at %s) on whether table %q has a sort key; one physical key schema must fit all entities",
					e.KeyPos, e.Name, ents[0].Name, ents[0].KeyPos, name)
			}
			sort.Slice(e.Patterns, func(i, j int) bool { return e.Patterns[i].Name < e.Patterns[j].Name })
			sort.Slice(e.Indexes, func(i, j int) bool { return e.Indexes[i].Name < e.Indexes[j].Name })
		}
		indexes, err := mergeIndexes(ents)
		if err != nil {
			return nil, err
		}
		t.Indexes = indexes
		s.Tables = append(s.Tables, t)
	}
	return s, nil
}

func mergeIndexes(ents []*Entity) ([]*Index, error) {
	merged := map[string]*Index{}
	owner := map[string]Pos{}
	for _, e := range ents {
		for _, ix := range e.Indexes {
			def := &Index{
				Name:       ix.Name,
				PKAttr:     PKAttrFor(ix.Name),
				Projection: ix.Projection,
			}
			if ix.Key.SK != nil {
				def.SKAttr = SKAttrFor(ix.Name)
			}
			prev, ok := merged[ix.Name]
			if !ok {
				merged[ix.Name] = def
				owner[ix.Name] = ix.Pos
				continue
			}
			if prev.Projection != def.Projection {
				return nil, fmt.Errorf("%s: GSI %q projection %q conflicts with projection %q declared at %s",
					ix.Pos, ix.Name, def.Projection, prev.Projection, owner[ix.Name])
			}
			if (prev.SKAttr != "") != (def.SKAttr != "") {
				return nil, fmt.Errorf("%s: GSI %q sort-key presence conflicts with the declaration at %s; every entity on a GSI must agree on whether it has a sort key",
					ix.Pos, ix.Name, owner[ix.Name])
			}
		}
	}
	names := make([]string, 0, len(merged))
	for n := range merged {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*Index, 0, len(names))
	for _, n := range names {
		out = append(out, merged[n])
	}
	return out, nil
}
