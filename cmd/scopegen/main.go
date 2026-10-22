// Command scopegen writes the Service operation-to-scopes map (ee/staff/service_scopes_gen.go)
// from the scopes the TypeSpec contract declares on each operation.
package main

import (
	"log"

	"github.com/getsphericon/sphericon/internal/scopegen"
)

func main() {
	if err := scopegen.Write("openapi/service.openapi.json", "ee/staff/service_scopes_gen.go"); err != nil {
		log.Fatal(err)
	}
}
