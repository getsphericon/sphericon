package suspension_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The owner is told that sending is frozen, why, and who did it, through the
// platform's system sender — which must keep working while the workspace itself is
// suspended (ADR 0007, 0015).
func TestNotifyOwnersEmailsOwners(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.System, "complaint rate above 0.3%")
	require.NoError(t, err)

	require.NoError(t, suspension.NotifyOwners(ctx, env.DB, accounts.OwnerNotifier(env.DB, env.SystemMail), "", fixtures.AcmeID))

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, fixtures.OwnerJohnEmail, msgs[0].To, "the workspace owner")
	assert.Contains(t, msgs[0].Subject, "Acme")
	assert.Contains(t, msgs[0].Text, "complaint rate above 0.3%", "the reason is stated")
	assert.Contains(t, msgs[0].Text, "automated abuse detection", "who set it is stated")
}

// A Staff member or the CLI appears to the customer as "Sphericon staff"; the Staff member's
// real id never leaves the platform.
func TestNotifyOwnersShowsStaffForStaffAndCLI(t *testing.T) {
	for name, by := range map[string]suspension.Actor{"staff": suspension.StaffMember("op-42"), "cli": suspension.CLI} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := context.Background()
			_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, by, "abuse report")
			require.NoError(t, err)

			require.NoError(t, suspension.NotifyOwners(ctx, env.DB, accounts.OwnerNotifier(env.DB, env.SystemMail), "", fixtures.AcmeID))

			msgs := env.SystemMail.Messages()
			require.Len(t, msgs, 1)
			assert.Contains(t, msgs[0].Text, "Set by: Sphericon staff")
			assert.NotContains(t, msgs[0].Text, "op-42")
		})
	}
}

func TestNotifyOwnersNilSenderIsNoop(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, suspension.NotifyOwners(context.Background(), env.DB, accounts.OwnerNotifier(env.DB, nil), "", fixtures.AcmeID))
}
