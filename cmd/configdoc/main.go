// Command configdoc writes the configuration reference (docs/configuration.md) from the config
// package: every environment variable Load reads, its default, rules and doc comment.
package main

import (
	"log"
	"os"

	"github.com/getsphericon/sphericon/config"
)

func main() {
	reference, err := config.Reference()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("docs/configuration.md", reference, 0o600); err != nil {
		log.Fatalf("write the configuration reference: %v", err)
	}
}
