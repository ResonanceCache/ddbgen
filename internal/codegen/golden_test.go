package codegen

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/ResonanceCache/ddbgen/internal/parser"
	"github.com/ResonanceCache/ddbgen/internal/schema"
)

var update = flag.Bool("update", false, "rewrite golden files")

func goldenCases(t *testing.T) []string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join("..", "..", "testdata", "codegen", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no codegen golden cases found")
	}
	return dirs
}

func compileCase(t *testing.T, dir string) *schema.Schema {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(dir, "input.go"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := parser.ParseSource("input.go", src)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestGolden compares generated output against testdata/codegen/<case>/expected,
// sqlc-style. Run with -update to regenerate.
func TestGolden(t *testing.T) {
	for _, dir := range goldenCases(t) {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			files, err := Generate(compileCase(t, dir))
			if err != nil {
				t.Fatal(err)
			}
			expDir := filepath.Join(dir, "expected")
			if *update {
				if err := os.RemoveAll(expDir); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(expDir, 0o755); err != nil {
					t.Fatal(err)
				}
				for name, content := range files {
					if err := os.WriteFile(filepath.Join(expDir, name), content, 0o644); err != nil {
						t.Fatal(err)
					}
				}
				return
			}
			expected, err := filepath.Glob(filepath.Join(expDir, "*.go"))
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			for _, path := range expected {
				name := filepath.Base(path)
				seen[name] = true
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				got, ok := files[name]
				if !ok {
					t.Errorf("expected file %s was not generated", name)
					continue
				}
				if string(got) != string(want) {
					t.Errorf("%s mismatch (rerun with -update to accept)\n--- got\n%s", name, got)
				}
			}
			for name := range files {
				if !seen[name] {
					t.Errorf("unexpected generated file %s (rerun with -update to accept)", name)
				}
			}
		})
	}
}
