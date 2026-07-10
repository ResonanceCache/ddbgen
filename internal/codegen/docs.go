package codegen

import (
	"fmt"
	"strings"

	"github.com/ResonanceCache/ddbgen/internal/schema"
)

// DocRow is one line of the access-pattern matrix. Rows are derived from
// the same views that drive code emission, so the matrix cannot drift from
// the generated methods.
type DocRow struct {
	Pattern      string
	Index        string
	KeyCondition string
	Returns      string
	Method       string
}

// DocRows renders the access-pattern matrix rows for one table: one row
// per //ddb:pattern plus one per item-collection partition.
func DocRows(t *schema.Table) ([]DocRow, error) {
	var rows []DocRow
	for _, e := range t.Entities {
		ev, err := buildEntityView(t, e)
		if err != nil {
			return nil, err
		}
		for i, pv := range ev.Patterns {
			p := e.Patterns[i]
			key, _ := e.KeyFor(p.Index)
			row := DocRow{
				Pattern:      p.Name,
				Index:        p.Index,
				KeyCondition: keyConditionDoc(p, key, pv),
				Returns:      fmt.Sprintf("[]%s (All iterator / Page)", e.Name),
				Method:       fmt.Sprintf("%s(%s)", pv.Name, argList(pv.Params)),
			}
			rows = append(rows, row)
		}
	}
	parts, err := buildPartitionViews(t)
	if err != nil {
		return nil, err
	}
	for _, pv := range parts {
		var members []string
		for _, m := range pv.Members {
			members = append(members, m.Plural)
		}
		rows = append(rows, DocRow{
			Pattern:      pv.Name,
			Index:        "main",
			KeyCondition: fmt.Sprintf("pk = %q", pv.PKRaw),
			Returns:      fmt.Sprintf("%s{%s, Unknown}", pv.Collection, strings.Join(members, ", ")),
			Method:       fmt.Sprintf("%s(%s).Collect(ctx)", pv.Name, argList(pv.Params)),
		})
	}
	return rows, nil
}

func keyConditionDoc(p *schema.Pattern, key schema.KeySpec, pv *patternView) string {
	cond := fmt.Sprintf("pk = %q", p.PK.Raw)
	switch pv.CondKind {
	case "eq":
		cond += fmt.Sprintf(" AND sk = %q", p.SKValue.Raw)
	case "begins":
		cond += fmt.Sprintf(" AND begins_with(sk, %q)", p.SKValue.Raw)
	case "implicit":
		cond += fmt.Sprintf(" AND begins_with(sk, %s)", pv.ValExpr)
	case "gt", "gte", "lt", "lte":
		op := map[string]string{"gt": ">", "gte": ">=", "lt": "<", "lte": "<="}[pv.CondKind]
		cond += fmt.Sprintf(" AND sk %s %q (entity-scoped)", op, p.SKValue.Raw)
	}
	if pv.Range != nil {
		cond += fmt.Sprintf(" — refinable via %[1]sAfter / %[1]sBefore / %[1]sBetween", pv.Range.Base)
	}
	return cond
}
