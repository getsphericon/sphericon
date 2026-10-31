// Command goverter is goverter with the enum transformer of internal/goverterenum, which matches
// enum members by value. `mise run generate:backend` runs it in place of the upstream command.
package main

import (
	"os"

	"github.com/jmattheis/goverter/cli"
	"github.com/jmattheis/goverter/enum"

	"github.com/getsphericon/sphericon/internal/goverterenum"
)

func main() {
	cli.Run(os.Args, cli.RunOpts{EnumTransformers: map[string]enum.Transformer{
		goverterenum.Name: goverterenum.ByValue,
	}})
}
