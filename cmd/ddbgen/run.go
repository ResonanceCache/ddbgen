package main

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/ResonanceCache/ddbgen/internal/analyze"
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
	for _, dir := range sortedDirs(byDir) {
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
		if err := generateInto(dir, sub, stdout); err != nil {
			return err
		}
		if err := schema.WriteSnapshot(filepath.Join(dir, schema.SnapshotName), sub); err != nil {
			return err
		}
		stdout.Printf("%s: wrote %s\n", dir, schema.SnapshotName)
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

// printer is the subset of cobra command output used by run functions.
type printer interface {
	Printf(format string, a ...any)
}
