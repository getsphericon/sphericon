package jobs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/jobs"
	"github.com/mokevnin/sphericon/internal/suspension"
	"github.com/mokevnin/sphericon/internal/testhelper"
)

// The owner is told that sending is frozen, why, and who did it, through the
// platform's system sender — which must keep working while the workspace itself is
// suspended (ADR 0007, 0015).
func TestNotifyWorkspaceSuspendedEmailsOwners(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.System, "complaint rate above 0.3%")
	require.NoError(t, err)

	require.NoError(t, jobs.NotifyWorkspaceSuspended(ctx, env.DB, env.SystemMail, fixtures.AcmeID))

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, fixtures.OwnerJohnEmail, msgs[0].To, "the workspace owner")
	assert.Contains(t, msgs[0].Subject, "Acme")
	assert.Contains(t, msgs[0].Text, "complaint rate above 0.3%", "the reason is stated")
	assert.Contains(t, msgs[0].Text, "automated abuse detection", "who set it is stated")
}

// An Operator or the CLI appears to the customer as "sphericon staff"; the Operator's
// real id never leaves the platform.
func TestNotifyWorkspaceSuspendedShowsStaffForOperatorAndCLI(t *testing.T) {
	for name, by := range map[string]suspension.Actor{"operator": suspension.Operator("op-42"), "cli": suspension.CLI} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := context.Background()
			_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, by, "abuse report")
			require.NoError(t, err)

			require.NoError(t, jobs.NotifyWorkspaceSuspended(ctx, env.DB, env.SystemMail, fixtures.AcmeID))

			msgs := env.SystemMail.Messages()
			require.Len(t, msgs, 1)
			assert.Contains(t, msgs[0].Text, "Set by: sphericon staff")
			assert.NotContains(t, msgs[0].Text, "op-42")
		})
	}
}

func TestNotifyWorkspaceSuspendedNilSenderIsNoop(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, jobs.NotifyWorkspaceSuspended(context.Background(), env.DB, nil, fixtures.AcmeID))
}
