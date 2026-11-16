// Command prcheck runs the pull request body checks (internal/prcheck) in CI: the body comes from
// SPHERICON_PR_BODY.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/getsphericon/sphericon/internal/prcheck"
)

const usage = `usage: the body is the SPHERICON_PR_BODY environment variable
  prcheck disclosure`

func main() {
	if err := run(os.Args[1:], os.Getenv("SPHERICON_PR_BODY")); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, body string) error {
	if len(args) != 1 || args[0] != "disclosure" {
		return errors.New(usage)
	}
	return prcheck.CheckDisclosure(body)
}
