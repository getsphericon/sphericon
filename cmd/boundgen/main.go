// Command boundgen writes the value bounds of the TypeSpec contract into the kernel package bounds
// (boundgen.Bounds), so the domain module that owns a field checks every write against them.
package main

import (
	"log"

	"github.com/getsphericon/sphericon/internal/boundgen"
)

func main() {
	if err := boundgen.Write(); err != nil {
		log.Fatal(err)
	}
}
