package sendingdomains_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// envelopeAt is the envelope of the Integration id in v; the test fails when there is none.
func envelopeAt(t *testing.T, v sendingdomains.View, id int64) sendingdomains.Envelope {
	t.Helper()
	e, ok := lo.Find(v.Envelopes, func(e sendingdomains.Envelope) bool { return e.IntegrationID == id })
	require.Truef(t, ok, "integration %d has an envelope", id)
	return e
}

// A later check replaces the envelope an earlier one read, whole: a custom MAIL FROM taken off the
// identity at SES reads as SES's own again, with nothing of the custom one left (ADR 0184).
func TestARecheckReplacesTheEnvelopeTheLastOneRead(t *testing.T) {
	env := testhelper.Setup(t)
	m := domains(env, dnstest.Resolver(t, nil).LookupTXT)
	env.SES.SetMailFrom(fixtures.SendingDomainUnverifiedDomain, messaging.MailFrom{
		Domain: "bounce.news.acme.com", Status: providers.MailFromSuccess, OnMXFailure: providers.MXFailureFallBack,
	})
	first, err := m.Verify(t.Context(), acme(env), fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	require.Equal(t, sendingdomains.EnvelopeCustom, envelopeAt(t, first, fixtures.IntegrationAcmeSesID).State)

	env.SES.SetMailFrom(fixtures.SendingDomainUnverifiedDomain, messaging.MailFrom{})
	again, err := m.Verify(t.Context(), acme(env), fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)

	ses := envelopeAt(t, again, fixtures.IntegrationAcmeSesID)
	assert.Equal(t, sendingdomains.EnvelopeProviderDefault, ses.State)
	assert.Nil(t, ses.MailFrom)
}

// The Test inbox sends nowhere, so a Workspace that keeps one gets no envelope for it, beside the
// envelopes of its other email Integrations.
func TestAKeptTestInboxHasNoEnvelope(t *testing.T) {
	env := testhelper.Setup(t)

	v, err := testhelper.Module[*sendingdomains.Module](env).Get(t.Context(), env.DB.Scoped(fixtures.GlobexID), fixtures.SendingDomainGlobexID)
	require.NoError(t, err)

	require.NotEmpty(t, v.Envelopes, "Globex's other email Integrations have envelopes")
	assert.Equal(t, providers.SES, envelopeAt(t, v, fixtures.IntegrationGlobexSesNoTopicID).Provider)
	_, inbox := lo.Find(v.Envelopes, func(e sendingdomains.Envelope) bool { return e.IntegrationID == fixtures.IntegrationGlobexInboxID })
	assert.False(t, inbox, "the kept Test inbox has no envelope")
}
