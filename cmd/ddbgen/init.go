package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var pkg string
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Scaffold a starter //ddb: annotated model file",
		Long: `Init writes ddb.go — a commented starter model with one entity, a GSI,
and two access patterns — into the given directory (default .). Edit the
templates, then run ddbgen generate.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			path := filepath.Join(dir, "ddb.go")
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists; refusing to overwrite", path)
			}
			if pkg == "" {
				pkg = filepath.Base(absOr(dir))
			}
			content := fmt.Sprintf(initTemplate, pkg)
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return err
			}
			cmd.Printf("wrote %s\nnext: edit the model, then run: ddbgen generate %s\n", path, dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&pkg, "package", "", "package name for the scaffold (default: directory name)")
	return cmd
}

func absOr(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}

const initTemplate = `// Package %[1]s holds a ddbgen single-table model.
//
// Markers describe the design; dynamodbav tags keep controlling attribute
// names and marshaling. Run "ddbgen generate ./..." after editing.
package %[1]s

import "time"

// Item is a starter entity. Rename it, adjust the key templates to your
// design, and add more entities that share the same table= name.
//
//ddb:entity table=app type=item version=Ver
//ddb:key pk="ITEM#{ID}" sk="ITEM#{ID}"
//ddb:index name=GSI1 pk="KIND#{Kind:upper}" sk="{CreatedAt:rfc3339}"
//ddb:pattern name=ItemsByKind index=GSI1 pk="KIND#{Kind:upper}"
type Item struct {
	ID        string    ` + "`dynamodbav:\"id\"`" + `
	Kind      string    ` + "`dynamodbav:\"kind\"`" + `
	Name      string    ` + "`dynamodbav:\"name\"`" + `
	CreatedAt time.Time ` + "`dynamodbav:\"created_at\"`" + `
	Ver       int64     ` + "`dynamodbav:\"v\"`" + `
}
`
