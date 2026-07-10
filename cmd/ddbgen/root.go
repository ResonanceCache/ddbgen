package main

import (
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// version is stamped by releases via -ldflags "-X main.version=v0.1.0";
// go-install builds fall back to module build info.
var version = ""

func resolveVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(unknown)"
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "ddbgen",
		Short: "Generate typed DynamoDB clients from annotated Go structs",
		Long: `ddbgen parses Go structs annotated with //ddb: marker comments describing
a single-table DynamoDB design (key templates, GSIs, access patterns) and
generates a fully typed client, infrastructure definitions, and an
access-pattern matrix document.`,
		Version:       resolveVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// Progress and reports belong on stdout; cobra defaults to stderr.
	root.SetOut(os.Stdout)
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
			cmd.Println("ddbgen", resolveVersion())
			return nil
		},
	}
}
