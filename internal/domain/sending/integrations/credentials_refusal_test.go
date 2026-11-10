package integrations_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A send the provider refused holds the Send route for the credentials it sent with only: one that
// lands after the Owner saved new credentials holds nothing, and they are tried at the next send
// (ADR 0109).
func TestARefusalOfReplacedCredentialsDoesNotHoldTheIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	id := int64(fixtures.IntegrationAcmeDefaultID)
	sentWith, err := s.Integration().Get(t.Context(), id)
	require.NoError(t, err)
	require.NoError(t, editSMTP(env, id, integrations.SMTPConfig{Host: "smtp.acme.test", Port: 587, Username: "u", Password: "new", From: "a@acme.test"}))

	err = integrations.RecordRefused(t.Context(), s, sentWith, time.Now())

	require.NoError(t, err)
	after, err := s.Integration().Get(t.Context(), id)
	require.NoError(t, err)
	assert.Nil(t, after.ProviderRefusedAt, "the refusal of the replaced credentials holds nothing")
}

// A refusal of the credentials the Integration still has holds it, and saving new ones lifts it.
func TestASaveOfNewCredentialsLiftsTheRefusal(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	id := int64(fixtures.IntegrationAcmeDefaultID)
	sentWith, err := s.Integration().Get(t.Context(), id)
	require.NoError(t, err)
	require.NoError(t, integrations.RecordRefused(t.Context(), s, sentWith, time.Now()))
	held, err := s.Integration().Get(t.Context(), id)
	require.NoError(t, err)
	require.NotNil(t, held.ProviderRefusedAt, "the refusal of the current credentials holds the Integration")

	require.NoError(t, editSMTP(env, id, integrations.SMTPConfig{Host: "smtp.acme.test", Port: 587, Username: "u", Password: "new", From: "a@acme.test"}))

	after, err := s.Integration().Get(t.Context(), id)
	require.NoError(t, err)
	assert.Nil(t, after.ProviderRefusedAt)
}
