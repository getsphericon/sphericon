// Command i18ngen writes the message ids of the backend catalog (internal/i18n/ids_gen.go) from
// internal/i18n/locales/en.json.
package main

import (
	"log"

	"github.com/getsphericon/sphericon/internal/i18n/idgen"
)

func main() {
	if err := idgen.Write("internal/i18n/locales/en.json", "internal/i18n/ids_gen.go"); err != nil {
		log.Fatal(err)
	}
}
