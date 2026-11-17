package integrations_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/integrationproviderstate"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A send the provider refused holds the Send route for the credentials it sent with only: one that
// lands after the Owner saved new credentials holds nothing, and they are tried at the next send
// (ADR 0109).
func TestARefusalOfReplacedCredentialsDoesNotHoldTheIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	id := int64(fixtures.IntegrationAcmeDefaultID)
	sentWith, err := s.Integration().QueryWithDiscarded().Where(integration.ID(id)).WithProviderState().Only(t.Context())
	require.NoError(t, err)
	require.NoError(t, editSMTP(env, id, integrations.SMTPConfig{Host: "smtp.acme.test", Port: 587, Username: "u", Password: "new", From: "a@acme.test"}))

	err = integrations.NewStanding(time.Now).RecordRefused(t.Context(), s, sentWith)

	require.NoError(t, err)
	after, err := s.Integration().QueryWithDiscarded().Where(integration.ID(id)).WithProviderState().Only(t.Context())
	require.NoError(t, err)
	assert.Nil(t, after.Edges.ProviderState.ProviderRefusedAt, "the refusal of the replaced credentials holds nothing")
}

// A refusal of the credentials the Integration still has holds it, and saving new ones lifts it.
func TestASaveOfNewCredentialsLiftsTheRefusal(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	id := int64(fixtures.IntegrationAcmeDefaultID)
	sentWith, err := s.Integration().QueryWithDiscarded().Where(integration.ID(id)).WithProviderState().Only(t.Context())
	require.NoError(t, err)
	require.NoError(t, integrations.NewStanding(time.Now).RecordRefused(t.Context(), s, sentWith))
	held, err := s.Integration().QueryWithDiscarded().Where(integration.ID(id)).WithProviderState().Only(t.Context())
	require.NoError(t, err)
	require.NotNil(t, held.Edges.ProviderState.ProviderRefusedAt, "the refusal of the current credentials holds the Integration")

	require.NoError(t, editSMTP(env, id, integrations.SMTPConfig{Host: "smtp.acme.test", Port: 587, Username: "u", Password: "new", From: "a@acme.test"}))

	after, err := s.Integration().QueryWithDiscarded().Where(integration.ID(id)).WithProviderState().Only(t.Context())
	require.NoError(t, err)
	assert.Nil(t, after.Edges.ProviderState.ProviderRefusedAt)
}

// A refusal that meets a save of new credentials in flight waits for it, and then holds nothing:
// the save wrote the provider state in its transaction (ADR 0168), so the refusal of the
// credentials it replaced misses once it commits.
func TestARefusalBehindASaveOfNewCredentialsHoldsNothing(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	s := env.DB.Scoped(fixtures.AcmeID)
	id := int64(fixtures.IntegrationAcmeDefaultID)
	sentWith, err := s.Integration().QueryWithDiscarded().Where(integration.ID(id)).WithProviderState().Only(t.Context())
	require.NoError(t, err)
	release := env.HoldOpen(t, s, func(ts *ent.Scoped, _ events.Publisher) error {
		if err := ts.Integration().UpdateOneID(id).SetConfigEncrypted(sealed(t, env, `{"host":"smtp.acme.test"}`)).Exec(t.Context()); err != nil {
			return err
		}
		return ts.IntegrationProviderState().Update().Where(integrationproviderstate.IntegrationID(id)).
			ClearProviderEnforcement().SetProviderSandbox(false).Exec(t.Context())
	})

	errs := env.QueueBehind(t, release, testhelper.Writer{Name: "refusal", Run: func() error {
		return integrations.NewStanding(time.Now).RecordRefused(t.Context(), s, sentWith)
	}})

	require.NoError(t, errs["refusal"])
	after, err := s.Integration().QueryWithDiscarded().Where(integration.ID(id)).WithProviderState().Only(t.Context())
	require.NoError(t, err)
	assert.Nil(t, after.Edges.ProviderState.ProviderRefusedAt, "the refusal of the replaced credentials holds nothing")
}

// sealed is plaintext sealed with the server's cipher, as a stored config is.
func sealed(t *testing.T, env *testhelper.TestEnv, plaintext string) string {
	t.Helper()
	out, err := env.Cipher.Encrypt([]byte(plaintext))
	require.NoError(t, err)
	return out
}
