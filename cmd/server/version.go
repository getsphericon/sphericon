package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/getsphericon/sphericon/internal/buildinfo"
)

// runVersion prints the build on one line (ADR 0034); --json prints the same as an object.
func runVersion(w io.Writer, args []string) error {
	info := buildinfo.Current()
	switch {
	case len(args) == 0:
		_, err := fmt.Fprintln(w, info.Line())
		return err
	case len(args) == 1 && args[0] == "--json":
		return json.NewEncoder(w).Encode(info)
	default:
		return fmt.Errorf("usage: sphericon version [--json]")
	}
}
