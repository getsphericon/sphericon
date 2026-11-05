// Command i18ngen writes the message ids of the backend catalog (internal/kernel/i18n/ids_gen.go) from
// internal/kernel/i18n/locales/en.json.
package main

import (
	"log"

	"github.com/getsphericon/sphericon/internal/kernel/i18n/idgen"
)

func main() {
	if err := idgen.Write("internal/kernel/i18n/locales/en.json", "internal/kernel/i18n/ids_gen.go"); err != nil {
		log.Fatal(err)
	}
}
