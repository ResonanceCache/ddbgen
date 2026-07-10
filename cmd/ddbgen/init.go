package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var pkg string
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Scaffold a starter //ddb: annotated model file",
		Long: `Init writes ddb.go — a commented starter model with one entity, a GSI,
and two access patterns — into the given directory (default .). The package
clause matches existing Go files in the directory, or a sanitized directory
name. Edit the templates, then run go generate (or ddbgen generate).`,
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
				pkg = detectPackage(dir)
			}
			content := fmt.Sprintf(initTemplate, pkg)
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return err
			}
			cmd.Printf("wrote %s\nnext: edit the model, then run: ddbgen generate %s\n", path, dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&pkg, "package", "", "package name for the scaffold (default: existing package in dir, else directory name)")
	return cmd
}

var identChars = regexp.MustCompile(`[^A-Za-z0-9_]`)

// detectPackage picks the scaffold's package clause: the package of
// existing Go files in the directory when there are any, else the
// directory name sanitized into a valid identifier.
func detectPackage(dir string) string {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.PackageClauseOnly)
	if err == nil {
		for name := range pkgs {
			if !strings.HasSuffix(name, "_test") {
				return name
			}
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	name := identChars.ReplaceAllString(filepath.Base(abs), "")
	if name == "" || name[0] >= '0' && name[0] <= '9' {
		name = "model"
	}
	return name
}

const initTemplate = `// Package %[1]s holds a ddbgen single-table model.
//
// Markers describe the design; dynamodbav tags keep controlling attribute
// names and marshaling. Regenerate with go generate (or ddbgen generate).
package %[1]s

//go:generate go run github.com/ResonanceCache/ddbgen/cmd/ddbgen generate .

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
