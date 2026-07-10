package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const genV1 = `package m

//ddb:entity table=app type=order
//ddb:key pk="ORDER#{ID}"
type Order struct {
	ID string ` + "`dynamodbav:\"id\"`" + `
}
`

// TestGenerateForceAndStaleCleanup drives the full generate flow in a temp
// module: first generate, breaking change blocked without --force, applied
// with it, and stale generated files removed after an entity rename.
func TestGenerateForceAndStaleCleanup(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module gentest\n\ngo 1.23\n")
	write("model.go", genV1)

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

	if err := runGenerate([]string{"."}, false, discard{}); err != nil {
		t.Fatalf("first generate: %v", err)
	}
	for _, f := range []string{"order_gen.go", "app_table_gen.go", "ddb.snapshot.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("missing %s after generate: %v", f, err)
		}
	}

	// Breaking template change: blocked without --force, applied with it.
	write("model.go", strings.Replace(genV1, `pk="ORDER#{ID}"`, `pk="ORD#{ID}"`, 1))
	err = runGenerate([]string{"."}, false, discard{})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("breaking change must be blocked without --force, got %v", err)
	}
	if err := runGenerate([]string{"."}, true, discard{}); err != nil {
		t.Fatalf("generate --force: %v", err)
	}
	if err := runDiff([]string{"."}, discard{}); err != nil {
		t.Fatalf("diff after --force must be clean: %v", err)
	}

	// Entity rename: the old generated file must not survive as an orphan,
	// and foreign files must be untouched.
	write("keepme_gen.go", "package m\n\n// hand-written, not ddbgen's\n")
	write("model.go", strings.ReplaceAll(genV1, "Order", "Invoice"))
	if err := runGenerate([]string{"."}, true, discard{}); err != nil {
		t.Fatalf("generate after rename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "order_gen.go")); !os.IsNotExist(err) {
		t.Fatal("stale order_gen.go survived the rename")
	}
	if _, err := os.Stat(filepath.Join(dir, "invoice_gen.go")); err != nil {
		t.Fatalf("invoice_gen.go missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "keepme_gen.go")); err != nil {
		t.Fatal("hand-written *_gen.go without the ddbgen header was removed")
	}
}
