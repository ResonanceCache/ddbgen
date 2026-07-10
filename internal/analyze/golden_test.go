package analyze

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/ResonanceCache/ddbgen/internal/parser"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestGolden runs the analyze corpus: each testdata/analyze/<case> holds an
// input.go (which must parse cleanly) and expected_issues.txt with one
// line per finding (empty for clean cases). Together the cases cover every
// DDB error code.
func TestGolden(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("..", "..", "testdata", "analyze", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no analyze golden cases found")
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(dir, "input.go"))
			if err != nil {
				t.Fatal(err)
			}
			s, err := parser.ParseSource("input.go", src)
			if err != nil {
				t.Fatalf("analyze corpus inputs must parse: %v", err)
			}
			got := ""
			if issues := Schema(s); len(issues) > 0 {
				got = issues.Error() + "\n"
			}
			goldenPath := filepath.Join(dir, "expected_issues.txt")
			if *update {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("issues mismatch (rerun with -update to accept)\n--- want\n%s--- got\n%s", want, got)
			}
		})
	}
}
