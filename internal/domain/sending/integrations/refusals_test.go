package integrations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Globex keeps its Test inbox: a second one breaks the index that keeps one per Workspace.
func TestCreateRefusesASecondTestInbox(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.Integrations.Create(t.Context(), env.DB.Scoped(fixtures.GlobexID), integrations.CreateInput{
		Name:   "Another inbox",
		Config: integrations.ConfigInput{Inbox: &integrations.InboxConfig{}},
	})

	require.ErrorIs(t, err, integrations.ErrOneInbox)
}

// A new default clears the channel's current one first; a default another request saves after
// that clear breaks the index that keeps one default per channel.
func TestCreateRefusesADefaultSavedMeanwhile(t *testing.T) {
	env := testhelper.Setup(t)
	env.Bus.Use(defaultSavedJustBefore())

	_, err := env.Integrations.Create(t.Context(), env.DB.Scoped(fixtures.AcmeID), integrations.CreateInput{
		Name:      "New default",
		IsDefault: true,
		Config: integrations.ConfigInput{SMTP: &integrations.SMTPConfig{
			Host: "smtp.example.com", Port: 587, Username: "user", Password: "secret", From: "news@example.com",
		}},
	})

	require.ErrorIs(t, err, integrations.ErrDefaultConflict)
}

// defaultSavedJustBefore is a hook of the bus that saves a default email Integration of Acme just
// before the first Integration created through it, in that transaction: the default a concurrent
// request saves between this one's clear of the old default and its insert. The save it makes runs
// through the hook too, so it fires once.
func defaultSavedJustBefore() ent.Hook {
	raced := false
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			im, ok := m.(*ent.IntegrationMutation)
			if ok && im.Op().Is(ent.OpCreate) && !raced {
				raced = true
				// test writes: the default a concurrent request saves between this one's clear and its insert
				im.Client().Integration.Create().SetWorkspaceID(fixtures.AcmeID).SetName("Saved meanwhile").
					SetChannel(channels.Email).SetProvider(providers.SMTP).SetConfigEncrypted("{}").
					SetIngestKey("omik_test_savedmeanwhile").SetIsDefault(true).ExecX(ctx)
			}
			return next.Mutate(ctx, m)
		})
	}
}
