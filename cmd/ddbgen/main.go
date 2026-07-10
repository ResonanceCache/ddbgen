// Command ddbgen generates typed Go DynamoDB clients, infrastructure
// definitions, and access-pattern documentation from marker-annotated
// Go structs describing a single-table design.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "ddbgen:", err)
		os.Exit(1)
	}
}
