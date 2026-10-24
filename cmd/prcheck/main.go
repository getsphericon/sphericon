package main

import (
	"errors"
	"fmt"
	"io"
	"os"
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
		return CheckDisclosure(body)
	case "markers":
		b, err := io.ReadAll(diff)
		if err != nil {
			return err
		}
		return CheckMarkers(AddedMarkerFiles(string(b)), body)
	default:
		return errors.New(usage)
	}
}
