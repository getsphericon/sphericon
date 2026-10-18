package main

import (
	"fmt"
	"io"

	"github.com/getsphericon/sphericon/internal/changelog"
)

// runUpgradeNotes prints the `Upgrade:` entries of every release after --since, from the
// changelog text it is given (the binary's embedded one), and nothing when there are none.
func runUpgradeNotes(w io.Writer, args []string, text string) error {
	if len(args) != 2 || args[0] != "--since" {
		return fmt.Errorf("usage: sphericon upgrade-notes --since <version>")
	}
	releases, err := changelog.Since(changelog.Parse(text), args[1])
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, changelog.Render(releases))
	return err
}
