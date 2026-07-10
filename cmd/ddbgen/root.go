package main

import (
	"github.com/spf13/cobra"
)

var version = "dev"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "ddbgen",
		Short: "Generate typed DynamoDB clients from annotated Go structs",
		Long: `ddbgen parses Go structs annotated with //ddb: marker comments describing
a single-table DynamoDB design (key templates, GSIs, access patterns) and
generates a fully typed client, infrastructure definitions, and an
access-pattern matrix document.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newGenerateCmd(),
		newDiffCmd(),
		newInfraCmd(),
		newDocsCmd(),
		newInitCmd(),
		newVersionCmd(),
	)
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print ddbgen version",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("ddbgen", version)
			return nil
		},
	}
}
