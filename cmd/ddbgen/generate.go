package main

import (
	"errors"

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
			return errors.New("not implemented")
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "allow breaking schema changes against the snapshot")
	return cmd
}

func newDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff [packages]",
		Short: "Check the schema against the committed snapshot (read-only)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented")
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
