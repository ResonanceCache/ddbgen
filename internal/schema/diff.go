package schema

import (
	"fmt"
	"strings"

	"github.com/ResonanceCache/ddbgen/internal/keytmpl"
)

// Change is one detected schema difference.
type Change struct {
	Breaking bool
	Msg      string
}

// DiffReport classifies schema changes against a snapshot.
type DiffReport struct {
	Changes []Change
}

// HasBreaking reports whether any change is breaking.
func (r *DiffReport) HasBreaking() bool {
	for _, c := range r.Changes {
		if c.Breaking {
			return true
		}
	}
	return false
}

// String renders the report, breaking changes first.
func (r *DiffReport) String() string {
	var b strings.Builder
	for _, c := range r.Changes {
		if c.Breaking {
			fmt.Fprintf(&b, "  breaking: %s\n", c.Msg)
		}
	}
	for _, c := range r.Changes {
		if !c.Breaking {
			fmt.Fprintf(&b, "  additive: %s\n", c.Msg)
		}
	}
	return b.String()
}

func (r *DiffReport) breaking(format string, a ...any) {
	r.Changes = append(r.Changes, Change{Breaking: true, Msg: fmt.Sprintf(format, a...)})
}

func (r *DiffReport) additive(format string, a ...any) {
	r.Changes = append(r.Changes, Change{Breaking: false, Msg: fmt.Sprintf(format, a...)})
}

// Diff compares a previously snapshotted schema (old) with a freshly
// compiled one (new). Breaking: changed key template structure, removed
// entity or pattern, changed physical attribute name, changed entity type
// string, changed entity-type attribute. Additive: new entity, new pattern,
// new GSI, new non-key field.
func Diff(oldS, newS *Schema) *DiffReport {
	r := &DiffReport{}
	oldTables := map[string]*Table{}
	for _, t := range oldS.Tables {
		oldTables[t.Name] = t
	}
	newTables := map[string]*Table{}
	for _, t := range newS.Tables {
		newTables[t.Name] = t
	}
	for name, ot := range oldTables {
		nt, ok := newTables[name]
		if !ok {
			r.breaking("table %q removed", name)
			continue
		}
		diffTable(r, ot, nt)
	}
	for name := range newTables {
		if _, ok := oldTables[name]; !ok {
			r.additive("new table %q", name)
		}
	}
	return r
}

func diffTable(r *DiffReport, ot, nt *Table) {
	oldIdx := map[string]*Index{}
	for _, ix := range ot.Indexes {
		oldIdx[ix.Name] = ix
	}
	for _, ix := range nt.Indexes {
		prev, ok := oldIdx[ix.Name]
		if !ok {
			r.additive("table %q: new GSI %q", nt.Name, ix.Name)
			continue
		}
		if prev.PKAttr != ix.PKAttr || prev.SKAttr != ix.SKAttr {
			r.breaking("table %q GSI %q: physical key attributes changed (%s/%s -> %s/%s)",
				nt.Name, ix.Name, prev.PKAttr, prev.SKAttr, ix.PKAttr, ix.SKAttr)
		}
		if prev.Projection != ix.Projection {
			r.breaking("table %q GSI %q: projection changed (%s -> %s)", nt.Name, ix.Name, prev.Projection, ix.Projection)
		}
		delete(oldIdx, ix.Name)
	}
	for name := range oldIdx {
		r.breaking("table %q: GSI %q removed", nt.Name, name)
	}

	oldEnts := map[string]*Entity{}
	for _, e := range ot.Entities {
		oldEnts[e.Name] = e
	}
	for _, e := range nt.Entities {
		prev, ok := oldEnts[e.Name]
		if !ok {
			r.additive("table %q: new entity %s", nt.Name, e.Name)
			continue
		}
		diffEntity(r, prev, e)
		delete(oldEnts, e.Name)
	}
	for name := range oldEnts {
		r.breaking("table %q: entity %s removed", nt.Name, name)
	}
}

func diffEntity(r *DiffReport, oe, ne *Entity) {
	if oe.Type != ne.Type {
		r.breaking("entity %s: type string changed (%q -> %q); existing items carry the old value", ne.Name, oe.Type, ne.Type)
	}
	if oe.ETAttr != ne.ETAttr {
		r.breaking("entity %s: entity-type attribute changed (%q -> %q)", ne.Name, oe.ETAttr, ne.ETAttr)
	}
	if oe.VersionField != ne.VersionField {
		r.breaking("entity %s: version field changed (%q -> %q); optimistic-locking semantics differ for existing items", ne.Name, oe.VersionField, ne.VersionField)
	}
	if oe.TTLField != ne.TTLField {
		r.breaking("entity %s: ttl field changed (%q -> %q)", ne.Name, oe.TTLField, ne.TTLField)
	}
	diffTemplate(r, ne.Name, "pk", rawOf(oe.Key.PK), rawOf(ne.Key.PK))
	diffTemplate(r, ne.Name, "sk", rawOf(oe.Key.SK), rawOf(ne.Key.SK))
	oldIx := map[string]EntityIndex{}
	for _, ix := range oe.Indexes {
		oldIx[ix.Name] = ix
	}
	for _, ix := range ne.Indexes {
		prev, ok := oldIx[ix.Name]
		if !ok {
			continue // table-level diff reports new GSIs
		}
		diffTemplate(r, ne.Name, ix.Name+" pk", rawOf(prev.Key.PK), rawOf(ix.Key.PK))
		diffTemplate(r, ne.Name, ix.Name+" sk", rawOf(prev.Key.SK), rawOf(ix.Key.SK))
		delete(oldIx, ix.Name)
	}
	for name := range oldIx {
		r.breaking("entity %s: GSI %q keys removed; items stop appearing in that index", ne.Name, name)
	}

	oldPats := map[string]*Pattern{}
	for _, p := range oe.Patterns {
		oldPats[p.Name] = p
	}
	for _, p := range ne.Patterns {
		prev, ok := oldPats[p.Name]
		if !ok {
			r.additive("entity %s: new pattern %s", ne.Name, p.Name)
			continue
		}
		if prev.Index != p.Index || rawOf(prev.PK) != rawOf(p.PK) || prev.SKCond != p.SKCond || prefixRaw(prev.SKValue) != prefixRaw(p.SKValue) {
			r.breaking("entity %s: pattern %s definition changed; callers of the generated method may silently read different items", ne.Name, p.Name)
		}
		delete(oldPats, p.Name)
	}
	for name := range oldPats {
		r.breaking("entity %s: pattern %s removed", ne.Name, name)
	}

	oldFields := map[string]Field{}
	for _, f := range oe.Fields {
		oldFields[f.Name] = f
	}
	for _, f := range ne.Fields {
		prev, ok := oldFields[f.Name]
		if !ok {
			r.additive("entity %s: new field %s", ne.Name, f.Name)
			continue
		}
		if prev.Attr != f.Attr {
			r.breaking("entity %s: field %s attribute renamed (%q -> %q); existing items keep the old attribute", ne.Name, f.Name, prev.Attr, f.Attr)
		}
		delete(oldFields, f.Name)
	}
	for name := range oldFields {
		r.additive("entity %s: field %s removed (existing items keep the attribute; new writes drop it)", ne.Name, name)
	}
}

func diffTemplate(r *DiffReport, entity, which, oldRaw, newRaw string) {
	if oldRaw != newRaw {
		r.breaking("entity %s: %s template changed (%q -> %q); keys of existing items no longer match", entity, which, oldRaw, newRaw)
	}
}

func rawOf(t *keytmpl.Template) string {
	if t == nil {
		return ""
	}
	return t.Raw
}

func prefixRaw(p *keytmpl.Prefix) string {
	if p == nil {
		return ""
	}
	return p.Raw
}
