package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedCodeBuilds writes each golden case's input plus generated
// output into a temporary module (with this repo substituted via a replace
// directive) and runs go build. Runs offline: dependency versions and sums
// are copied from the repo module, so everything resolves from the local
// module cache.
func TestGeneratedCodeBuilds(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	goMod, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	goSum, err := os.ReadFile(filepath.Join(repoRoot, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	tmpGoMod := strings.Replace(string(goMod),
		"module github.com/ResonanceCache/ddbgen",
		"module ddbgen_buildtest", 1)
	tmpGoMod += "\nrequire github.com/ResonanceCache/ddbgen v0.0.0-00010101000000-000000000000\n" +
		"replace github.com/ResonanceCache/ddbgen => " + repoRoot + "\n"

	for _, dir := range goldenCases(t) {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			files, err := Generate(compileCase(t, dir))
			if err != nil {
				t.Fatal(err)
			}
			tmp := t.TempDir()
			write := func(name string, data []byte) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(tmp, name), data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write("go.mod", []byte(tmpGoMod))
			write("go.sum", goSum)
			input, err := os.ReadFile(filepath.Join(dir, "input.go"))
			if err != nil {
				t.Fatal(err)
			}
			write("input.go", input)
			for name, content := range files {
				write(name, content)
			}
			cmd := exec.Command("go", "build", "./...")
			cmd.Dir = tmp
			cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("generated code does not build: %v\n%s", err, out)
			}
			vet := exec.Command("go", "vet", "./...")
			vet.Dir = tmp
			vet.Env = cmd.Env
			if out, err := vet.CombinedOutput(); err != nil {
				t.Fatalf("go vet on generated code: %v\n%s", err, out)
			}
		})
	}
}
