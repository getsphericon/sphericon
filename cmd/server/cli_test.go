package main

import (
	"context"
	"io"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/ee/staff"
	"github.com/getsphericon/sphericon/ent"
)

// testOps is commandOps built from whichever fakes a test supplies; the others stay nil.
type testOps struct {
	workspaceOps
	userOps
	staffOps
	serviceCredentialOps
	rekeyOps
}

var cliNow = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

// execute runs `sphericon <args>` through the real command tree against the fake ops
// (a workspaceOps, userOps, staffOps or serviceCredentialOps), writing results to out.
func execute(fake any, out io.Writer, args ...string) error {
	return executeAt(cliNow, fake, out, args...)
}

// executeAt is execute with the command clock at `now`, for commands whose real operation
// compares the clock with stored times.
func executeAt(now time.Time, fake any, out io.Writer, args ...string) error {
	var ops testOps
	ops.workspaceOps, _ = fake.(workspaceOps)
	ops.userOps, _ = fake.(userOps)
	ops.staffOps, _ = fake.(staffOps)
	ops.serviceCredentialOps, _ = fake.(serviceCredentialOps)
	ops.rekeyOps, _ = fake.(rekeyOps)

	return runDeps(deps{
		env: "test",
		now: func() time.Time { return now },
		open: func(context.Context) (commandOps, func(), error) {
			return ops, func() {}, nil
		},
	}, out, args...)
}

// runDeps runs `sphericon <args>` through the real command tree built on d, writing results to out.
func runDeps(d deps, out io.Writer, args ...string) error {
	root := newRoot(d)
	root.Writer = out
	root.ErrWriter = io.Discard
	root.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	return root.Run(context.Background(), append([]string{"sphericon"}, args...))
}

// staffCLI is the real Staff module behind the `staff` and `service-credential` commands, so a
// test observes what the command left in the database.
type staffCLI struct{ m *staff.Module }

func (c staffCLI) CreateStaffMember(ctx context.Context, email string) (string, error) {
	return c.m.Create(ctx, email)
}

func (c staffCLI) ResetStaffTOTP(ctx context.Context, email string) error {
	return c.m.ResetTOTP(ctx, email)
}

func (c staffCLI) IssueServiceCredential(ctx context.Context, in staff.IssueInput) (*staff.Issued, error) {
	return c.m.IssueServiceCredential(ctx, in)
}

func (c staffCLI) RotateServiceCredential(ctx context.Context, id int64, expiresAt *time.Time) (*staff.Issued, error) {
	return c.m.RotateServiceCredential(ctx, id, expiresAt)
}

func (c staffCLI) RevokeServiceCredential(ctx context.Context, id int64) error {
	return c.m.RevokeServiceCredential(ctx, id)
}

func (c staffCLI) ServiceCredentials(ctx context.Context, name string) ([]*ent.ServiceCredential, error) {
	return c.m.ServiceCredentials(ctx, name)
}
