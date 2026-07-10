package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/ResonanceCache/ddbgen/internal/codegen"
	"github.com/ResonanceCache/ddbgen/internal/schema"
	"github.com/spf13/cobra"
)

func newGenerateCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "generate [packages]",
		Short: "Generate typed clients from //ddb: annotated structs",
		Long: `Generate parses the given package patterns (like go vet: ./... etc.),
compiles the marker schema, runs static analysis, and emits generated Go
files next to the annotated source. A schema snapshot (ddb.snapshot.json)
is written alongside; breaking schema changes against an existing snapshot
fail the run unless --force is given.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(args, force, cmd)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "allow breaking schema changes against the snapshot")
	return cmd
}

func newDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff [packages]",
		Short: "Check the schema against the committed snapshot (read-only)",
		Long: `Diff recompiles the schema and compares it with the committed
` + schema.SnapshotName + ` without writing anything. Additive changes are
reported; breaking changes exit nonzero. Intended for CI.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiff(args, cmd)
		},
	}
}

func newInfraCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "infra [packages]",
		Short: "Emit infrastructure definitions (CloudFormation or Terraform)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented")
		},
	}
	cmd.Flags().StringVar(&format, "format", "cfn", "output format: cfn or tf")
	return cmd
}

func newDocsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "docs [packages]",
		Short: "Emit ACCESS_PATTERNS.md access-pattern matrix",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented")
		},
	}
}

// generateInto renders and writes all generated files for one directory.
func generateInto(dir string, sub *schema.Schema, stdout printer) error {
	files, err := codegen.Generate(sub)
	if err != nil {
		return err
	}
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
	return nil
}
