// Command scopegen writes the access tables of the API surfaces (scopegen.Tables) from what the
// TypeSpec contract declares on each operation.
package main

import (
	"log"

	"github.com/getsphericon/sphericon/internal/scopegen"
)

func main() {
	for i := range scopegen.Tables {
		if err := scopegen.Tables[i].Write(); err != nil {
			log.Fatal(err)
		}
	}
}
