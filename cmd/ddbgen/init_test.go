package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ResonanceCache/ddbgen/internal/parser"
)

// TestInitScaffoldParses proves the init scaffold is a valid ddbgen model.
func TestInitScaffoldParses(t *testing.T) {
	dir := t.TempDir()
	root := newRootCmd()
	root.SetArgs([]string{"init", "--package", "store", dir})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(dir, "ddb.go"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := parser.ParseSource("ddb.go", src)
	if err != nil {
		t.Fatalf("scaffold must parse: %v", err)
	}
	if len(s.Tables) != 1 || len(s.Tables[0].Entities) != 1 {
		t.Fatalf("unexpected scaffold schema: %+v", s.Tables)
	}

	// Refuses to overwrite.
	root = newRootCmd()
	root.SetArgs([]string{"init", dir})
	if err := root.Execute(); err == nil {
		t.Fatal("init must refuse to overwrite an existing ddb.go")
	}
}
