package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ResonanceCache/ddbgen/internal/parser"
	"github.com/ResonanceCache/ddbgen/internal/schema"
)

const diffV1 = `package m

import "time"

//ddb:entity table=app type=order
//ddb:key pk="TENANT#{TenantID}" sk="ORDER#{CreatedAt:rfc3339}#{OrderID}"
type Order struct {
	TenantID  string    ` + "`dynamodbav:\"tenant_id\"`" + `
	OrderID   string    ` + "`dynamodbav:\"order_id\"`" + `
	CreatedAt time.Time ` + "`dynamodbav:\"created_at\"`" + `
}
`

type discard struct{}

func (discard) Printf(string, ...any) {}

// TestDiffCatchesBreakingChange seeds a snapshot, changes a key template's
// encoder, and proves ddbgen diff fails.
func TestDiffCatchesBreakingChange(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module difftest\n\ngo 1.23\n")
	write("model.go", diffV1)

	s, err := parser.ParseSource("model.go", []byte(diffV1))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.WriteSnapshot(filepath.Join(dir, schema.SnapshotName), s); err != nil {
		t.Fatal(err)
	}

	// Package loading resolves patterns against the working directory's
	// module, so run from inside the temp module.
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatal(err)
		}
	}()

	// Unchanged schema passes.
	if err := runDiff([]string{"."}, discard{}); err != nil {
		t.Fatalf("identical schema must pass diff: %v", err)
	}

	// A changed sk encoder is a breaking key-template change.
	write("model.go", strings.Replace(diffV1, "CreatedAt:rfc3339", "CreatedAt:epoch", 1))
	err = runDiff([]string{"."}, discard{})
	if err == nil {
		t.Fatal("diff must fail on a breaking key-template change")
	}
	if !strings.Contains(err.Error(), "breaking") {
		t.Fatalf("unexpected diff error: %v", err)
	}
}
