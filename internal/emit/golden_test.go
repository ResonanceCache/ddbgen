package emit

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/ResonanceCache/ddbgen/internal/parser"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestGolden renders infra and docs output for each testdata/emit case and
// compares against expected/ files.
func TestGolden(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("..", "..", "testdata", "emit", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no emit golden cases found")
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(dir, "input.go"))
			if err != nil {
				t.Fatal(err)
			}
			s, err := parser.ParseSource("input.go", src)
			if err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{}
			for _, tbl := range s.Tables {
				cfn, err := CloudFormation(tbl)
				if err != nil {
					t.Fatal(err)
				}
				files["table_"+tbl.Name+".cfn.yaml"] = cfn
				tf, err := Terraform(tbl)
				if err != nil {
					t.Fatal(err)
				}
				files["table_"+tbl.Name+".tf"] = tf
			}
			docs, err := AccessPatterns(s)
			if err != nil {
				t.Fatal(err)
			}
			files["ACCESS_PATTERNS.md"] = docs

			expDir := filepath.Join(dir, "expected")
			if *update {
				if err := os.RemoveAll(expDir); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(expDir, 0o755); err != nil {
					t.Fatal(err)
				}
				for name, data := range files {
					if err := os.WriteFile(filepath.Join(expDir, name), data, 0o644); err != nil {
						t.Fatal(err)
					}
				}
				return
			}
			for name, got := range files {
				want, err := os.ReadFile(filepath.Join(expDir, name))
				if err != nil {
					t.Fatalf("%s: %v (rerun with -update)", name, err)
				}
				if string(got) != string(want) {
					t.Errorf("%s mismatch (rerun with -update to accept)\n--- got\n%s", name, got)
				}
			}
		})
	}
}
