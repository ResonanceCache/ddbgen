package parser

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestGolden runs the parser corpus: each testdata/parser/<case> holds an
// input.go plus either expected_ir.json or expected_err.txt.
func TestGolden(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("..", "..", "testdata", "parser", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no parser golden cases found")
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(dir, "input.go"))
			if err != nil {
				t.Fatal(err)
			}
			s, perr := ParseSource("input.go", src)

			errPath := filepath.Join(dir, "expected_err.txt")
			irPath := filepath.Join(dir, "expected_ir.json")

			if *update {
				if perr != nil {
					writeFile(t, errPath, []byte(perr.Error()+"\n"))
					_ = os.Remove(irPath)
					return
				}
				writeFile(t, irPath, marshalIR(t, s))
				_ = os.Remove(errPath)
				return
			}

			if _, statErr := os.Stat(errPath); statErr == nil {
				want := readFile(t, errPath)
				if perr == nil {
					t.Fatalf("expected error:\n%s\ngot none", want)
				}
				if got := perr.Error() + "\n"; got != string(want) {
					t.Errorf("error mismatch\n--- want\n%s--- got\n%s", want, got)
				}
				return
			}
			if perr != nil {
				t.Fatalf("unexpected error: %v", perr)
			}
			want := readFile(t, irPath)
			if got := marshalIR(t, s); string(got) != string(want) {
				t.Errorf("IR mismatch (rerun with -update to accept)\n--- want\n%s--- got\n%s", want, got)
			}
		})
	}
}

func marshalIR(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
