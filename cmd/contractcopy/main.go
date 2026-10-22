// Command contractcopy fails when Go, TypeScript or a fixture repeats an enum, a limit or a scope
// that a TypeSpec declaration already holds (rule 6, docs/agents/review-rules.md).
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/getsphericon/sphericon/internal/contractcopy"
)

func main() {
	findings, err := contractcopy.Check(".", "openapi")
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range findings {
		fmt.Fprintln(os.Stderr, f)
	}
	if len(findings) > 0 {
		os.Exit(1)
	}
}
