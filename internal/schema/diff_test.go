package schema_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ResonanceCache/ddbgen/internal/parser"
	"github.com/ResonanceCache/ddbgen/internal/schema"
)

const baseSrc = `package app

import "time"

//ddb:entity table=app type=order version=Ver
//ddb:key pk="TENANT#{TenantID}" sk="ORDER#{CreatedAt:rfc3339}#{OrderID}"
//ddb:index name=GSI1 pk="STATUS#{Status:upper}" sk="{UpdatedAt:epoch}"
//ddb:pattern name=OrdersByTenant index=main pk="TENANT#{TenantID}" sk.begins="ORDER#"
type Order struct {
	TenantID  string    ` + "`dynamodbav:\"tenant_id\"`" + `
	OrderID   string    ` + "`dynamodbav:\"order_id\"`" + `
	Status    string    ` + "`dynamodbav:\"status\"`" + `
	CreatedAt time.Time ` + "`dynamodbav:\"created_at\"`" + `
	UpdatedAt time.Time ` + "`dynamodbav:\"updated_at\"`" + `
	Ver       int64     ` + "`dynamodbav:\"v\"`" + `
}
`

func compile(t *testing.T, src string) *schema.Schema {
	t.Helper()
	s, err := parser.ParseSource("input.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDiffClassification(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(string) string
		breaking string // substring of a required breaking change, "" if none
		additive string // substring of a required additive change, "" if none
	}{
		{
			name:   "identical",
			mutate: func(s string) string { return s },
		},
		{
			name:     "changed sk encoder",
			mutate:   func(s string) string { return strings.Replace(s, "CreatedAt:rfc3339", "CreatedAt:epoch", 1) },
			breaking: "sk template changed",
		},
		{
			name: "removed pattern",
			mutate: func(s string) string {
				return strings.Replace(s, "//ddb:pattern name=OrdersByTenant index=main pk=\"TENANT#{TenantID}\" sk.begins=\"ORDER#\"\n", "", 1)
			},
			breaking: "pattern OrdersByTenant removed",
		},
		{
			name:     "changed entity type string",
			mutate:   func(s string) string { return strings.Replace(s, "type=order", "type=orders", 1) },
			breaking: "type string changed",
		},
		{
			name:     "changed et attribute",
			mutate:   func(s string) string { return strings.Replace(s, "type=order", "type=order et=kind", 1) },
			breaking: "entity-type attribute changed",
		},
		{
			name:     "renamed GSI (physical attribute change)",
			mutate:   func(s string) string { return strings.ReplaceAll(s, "GSI1", "ByStatus") },
			breaking: `GSI "GSI1" removed`,
			additive: `new GSI "ByStatus"`,
		},
		{
			name:     "changed field attribute name",
			mutate:   func(s string) string { return strings.Replace(s, `dynamodbav:"status"`, `dynamodbav:"state"`, 1) },
			breaking: "attribute renamed",
		},
		{
			name:     "changed version field",
			mutate:   func(s string) string { return strings.Replace(s, "version=Ver", "", 1) },
			breaking: "version field changed",
		},
		{
			name: "new pattern",
			mutate: func(s string) string {
				return strings.Replace(s, "//ddb:pattern",
					"//ddb:pattern name=OrdersByStatus index=GSI1 pk=\"STATUS#{Status:upper}\"\n//ddb:pattern", 1)
			},
			additive: "new pattern OrdersByStatus",
		},
		{
			name:     "new field",
			mutate:   func(s string) string { return strings.Replace(s, "\tVer ", "\tNote string\n\tVer ", 1) },
			additive: "new field Note",
		},
		{
			name: "new entity",
			mutate: func(s string) string {
				return s + `
//ddb:entity table=app type=payment
//ddb:key pk="TENANT#{TenantID}" sk="PAY#{PayID}"
type Payment struct {
	TenantID string
	PayID    string
}
`
			},
			additive: "new entity Payment",
		},
	}
	oldS := compile(t, baseSrc)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newS := compile(t, tt.mutate(baseSrc))
			r := schema.Diff(oldS, newS)
			if tt.breaking == "" && r.HasBreaking() {
				t.Fatalf("unexpected breaking changes:\n%s", r)
			}
			if tt.breaking != "" && !containsChange(r, true, tt.breaking) {
				t.Fatalf("missing breaking change containing %q, got:\n%s", tt.breaking, r)
			}
			if tt.additive != "" && !containsChange(r, false, tt.additive) {
				t.Fatalf("missing additive change containing %q, got:\n%s", tt.additive, r)
			}
			if tt.breaking == "" && tt.additive == "" && len(r.Changes) != 0 {
				t.Fatalf("expected no changes, got:\n%s", r)
			}
		})
	}
}

func containsChange(r *schema.DiffReport, breaking bool, substr string) bool {
	for _, c := range r.Changes {
		if c.Breaking == breaking && strings.Contains(c.Msg, substr) {
			return true
		}
	}
	return false
}

func TestSnapshotRoundTrip(t *testing.T) {
	s := compile(t, baseSrc)
	path := filepath.Join(t.TempDir(), schema.SnapshotName)
	if err := schema.WriteSnapshot(path, s); err != nil {
		t.Fatal(err)
	}
	back, err := schema.ReadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if back == nil {
		t.Fatal("snapshot read back nil")
	}
	if r := schema.Diff(back, s); len(r.Changes) != 0 {
		t.Fatalf("round-trip produced diffs:\n%s", r)
	}
}

func TestReadSnapshotMissing(t *testing.T) {
	s, err := schema.ReadSnapshot(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || s != nil {
		t.Fatalf("want (nil, nil) for missing snapshot, got (%v, %v)", s, err)
	}
}
