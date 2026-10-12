// Command fixturegen writes the fixture catalog (internal/fixtures/catalog_gen.go)
// from the annotated rows of fixtures/*.yml.
package main

import (
	"log"

	"github.com/getsphericon/sphericon/internal/fixtures/fixturegen"
)

func main() {
	if err := fixturegen.Write("fixtures", "internal/fixtures/catalog_gen.go"); err != nil {
		log.Fatal(err)
	}
}
