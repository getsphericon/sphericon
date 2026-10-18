package main

import (
	"context"
	"io"
	"time"

	"github.com/urfave/cli/v3"
)

// testOps is commandOps built from whichever fakes a test supplies; the others stay nil.
type testOps struct {
	workspaceOps
	userOps
	staffOps
	serviceCredentialOps
}

var cliNow = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

// execute runs `sphericon <args>` through the real command tree against the fake ops
// (a workspaceOps, userOps, staffOps or serviceCredentialOps), writing results to out.
func execute(fake any, out io.Writer, args ...string) error {
	var ops testOps
	ops.workspaceOps, _ = fake.(workspaceOps)
	ops.userOps, _ = fake.(userOps)
	ops.staffOps, _ = fake.(staffOps)
	ops.serviceCredentialOps, _ = fake.(serviceCredentialOps)

	root := newRoot(deps{
		env: "test",
		now: func() time.Time { return cliNow },
		open: func(context.Context) (commandOps, func(), error) {
			return ops, func() {}, nil
		},
	})
	root.Writer = out
	root.ErrWriter = io.Discard
	root.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	return root.Run(context.Background(), append([]string{"sphericon"}, args...))
}
