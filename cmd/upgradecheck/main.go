// Command upgradecheck runs the Upgrade checks of ADR 0033 (internal/upgradecheck) over a pull
// request in CI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/getsphericon/sphericon/internal/upgradecheck"
)

const usage = `usage:
  upgradecheck breaking   --title T [--body-file F] --base REF
  upgradecheck migrations --title T --base REF
  upgradecheck immutable  [--body-file F] --base REF`

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	title := fs.String("title", "", "pull request title (the squash commit subject)")
	bodyFile := fs.String("body-file", "", "file with the pull request body")
	base := fs.String("base", "origin/main", "ref the pull request is compared against")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	switch args[0] {
	case "breaking":
		body, err := readBody(*bodyFile)
		if err != nil {
			return err
		}
		return upgradecheck.Breaking(ctx, *title, body, *base)
	case "migrations":
		linted, err := upgradecheck.Migrations(ctx, *title, *base)
		if err != nil {
			return err
		}
		if len(linted) == 0 {
			fmt.Println("no migration added; nothing to lint")
		}
		return nil
	case "immutable":
		body, err := readBody(*bodyFile)
		if err != nil {
			return err
		}
		return upgradecheck.Immutable(ctx, body, *base)
	default:
		return errors.New(usage)
	}
}

// readBody reads the pull request body from the file the flag names, or none when it names none.
func readBody(bodyFile string) (string, error) {
	if bodyFile == "" {
		return "", nil
	}
	b, err := os.ReadFile(bodyFile)
	return string(b), err
}
