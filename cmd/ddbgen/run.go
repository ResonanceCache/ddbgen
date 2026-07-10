package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ResonanceCache/ddbgen/internal/analyze"
	"github.com/ResonanceCache/ddbgen/internal/codegen"
	"github.com/ResonanceCache/ddbgen/internal/emit"
	"github.com/ResonanceCache/ddbgen/internal/parser"
	"github.com/ResonanceCache/ddbgen/internal/schema"
)

// schemasByDir splits a loaded schema into one sub-schema per output
// directory, so each annotated package gets its own snapshot and
// generated files.
func schemasByDir(s *schema.Schema) map[string]*schema.Schema {
	out := map[string]*schema.Schema{}
	for _, t := range s.Tables {
		sub, ok := out[t.Dir]
		if !ok {
			sub = &schema.Schema{}
			out[t.Dir] = sub
		}
		sub.Tables = append(sub.Tables, t)
	}
	return out
}

func sortedDirs(m map[string]*schema.Schema) []string {
	dirs := make([]string, 0, len(m))
	for d := range m {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs
}

// checkSnapshot diffs the compiled schema for one directory against its
// snapshot. It returns the report (nil when no snapshot exists yet).
func checkSnapshot(dir string, sub *schema.Schema) (*schema.DiffReport, error) {
	old, err := schema.ReadSnapshot(filepath.Join(dir, schema.SnapshotName))
	if err != nil {
		return nil, err
	}
	if old == nil {
		return nil, nil
	}
	return schema.Diff(old, sub), nil
}

func loadSchemas(args []string) (map[string]*schema.Schema, error) {
	s, err := parser.Load(args...)
	if err != nil {
		return nil, err
	}
	if issues := analyze.Schema(s); len(issues) > 0 {
		return nil, issues
	}
	return schemasByDir(s), nil
}

func runGenerate(args []string, force bool, stdout printer) error {
	byDir, err := loadSchemas(args)
	if err != nil {
		return err
	}
	// Two phases: validate and render everything first, then write. A
	// failure in one package must not leave earlier packages regenerated
	// with advanced snapshots.
	dirs := sortedDirs(byDir)
	rendered := make(map[string]map[string][]byte, len(dirs))
	for _, dir := range dirs {
		sub := byDir[dir]
		report, err := checkSnapshot(dir, sub)
		if err != nil {
			return err
		}
		if report != nil && len(report.Changes) > 0 {
			stdout.Printf("%s: schema changes since snapshot:\n%s", dir, report)
			if report.HasBreaking() && !force {
				return fmt.Errorf("breaking schema changes detected in %s (rerun with --force to accept)", dir)
			}
		}
		files, err := codegen.Generate(sub)
		if err != nil {
			return err
		}
		rendered[dir] = files
	}
	for _, dir := range dirs {
		if err := writeGenerated(dir, rendered[dir], stdout); err != nil {
			return err
		}
		if err := schema.WriteSnapshot(filepath.Join(dir, schema.SnapshotName), byDir[dir]); err != nil {
			return err
		}
		stdout.Printf("%s: wrote %s\n", dir, schema.SnapshotName)
	}
	return nil
}

// writeGenerated writes the rendered files and removes stale *_gen.go
// files from earlier runs (identified by the generated-code header), so a
// renamed or removed entity cannot leave an uncompilable orphan behind.
func writeGenerated(dir string, files map[string][]byte, stdout printer) error {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), files[name], 0o644); err != nil {
			return err
		}
		stdout.Printf("%s: wrote %s\n", dir, name)
	}
	existing, err := filepath.Glob(filepath.Join(dir, "*_gen.go"))
	if err != nil {
		return err
	}
	for _, path := range existing {
		if _, current := files[filepath.Base(path)]; current {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(string(content), codegen.Header) {
			continue // not ours; leave it alone
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		stdout.Printf("%s: removed stale %s\n", dir, filepath.Base(path))
	}
	return nil
}

func runDiff(args []string, stdout printer) error {
	byDir, err := loadSchemas(args)
	if err != nil {
		return err
	}
	for _, dir := range sortedDirs(byDir) {
		report, err := checkSnapshot(dir, byDir[dir])
		if err != nil {
			return err
		}
		if report == nil {
			return fmt.Errorf("%s: no %s found; run ddbgen generate and commit the snapshot", dir, schema.SnapshotName)
		}
		if len(report.Changes) > 0 {
			stdout.Printf("%s: schema changes since snapshot:\n%s", dir, report)
		}
		if report.HasBreaking() {
			return fmt.Errorf("breaking schema changes detected in %s", dir)
		}
	}
	stdout.Printf("no breaking schema changes\n")
	return nil
}

func runInfra(args []string, format, outDir string, stdout printer) error {
	byDir, err := loadSchemas(args)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for _, dir := range sortedDirs(byDir) {
		for _, t := range byDir[dir].Tables {
			var data []byte
			var name string
			switch format {
			case "cfn":
				name = "table_" + t.Name + ".cfn.yaml"
				data, err = emit.CloudFormation(t)
			case "tf":
				name = "table_" + t.Name + ".tf"
				data, err = emit.Terraform(t)
			}
			if err != nil {
				return err
			}
			path := filepath.Join(outDir, name)
			if err := os.WriteFile(path, data, 0o644); err != nil {
				return err
			}
			stdout.Printf("wrote %s\n", path)
		}
	}
	return nil
}

func runDocs(args []string, stdout printer) error {
	byDir, err := loadSchemas(args)
	if err != nil {
		return err
	}
	for _, dir := range sortedDirs(byDir) {
		data, err := emit.AccessPatterns(byDir[dir])
		if err != nil {
			return err
		}
		path := filepath.Join(dir, "ACCESS_PATTERNS.md")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		stdout.Printf("wrote %s\n", path)
	}
	return nil
}

// printer is the subset of cobra command output used by run functions.
type printer interface {
	Printf(format string, a ...any)
}
