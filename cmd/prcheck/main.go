// Command prcheck runs the pull request body checks (internal/prcheck) in CI: the body comes from
// PR_BODY and the diff of the markers check from stdin.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/getsphericon/sphericon/internal/prcheck"
)

const usage = `usage: the body is the PR_BODY environment variable
  prcheck disclosure
  git diff --unified=0 BASE...HEAD | prcheck markers`

func main() {
	if err := run(os.Args[1:], os.Getenv("PR_BODY"), os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, body string, diff io.Reader) error {
	if len(args) != 1 {
		return errors.New(usage)
	}
	switch args[0] {
	case "disclosure":
		return prcheck.CheckDisclosure(body)
	case "markers":
		b, err := io.ReadAll(diff)
		if err != nil {
			return err
		}
		return prcheck.CheckMarkers(prcheck.AddedMarkerFiles(string(b)), body)
	default:
		return errors.New(usage)
	}
}
