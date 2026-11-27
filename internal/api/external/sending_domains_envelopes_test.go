package external_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// externalEnvelope is the envelope of the Integration id among envs; the test fails when there is none.
func externalEnvelope(t *testing.T, envs []externalapi.SendingDomainEnvelope, id int64) externalapi.SendingDomainEnvelope {
	t.Helper()
	e, ok := lo.Find(envs, func(e externalapi.SendingDomainEnvelope) bool { return e.IntegrationId == entityIDString(id) })
	require.Truef(t, ok, "integration %d has an envelope", id)
	return e
}

// The /api Sending domain carries the envelope of each email Integration, as its last check read
// it (ADR 0184): what an agent reads through sending_domains_list.
func TestExternalSendingDomainsListShowsTheEnvelopes(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeSendingDomainsRead)

	page, err := c.SendingDomainsList(t.Context(), externalapi.SendingDomainsListParams{})
	require.NoError(t, err)

	verified, found := lo.Find(page.Items, func(d externalapi.SendingDomainResource) bool {
		return d.ID == entityIDString(fixtures.SendingDomainVerifiedID)
	})
	require.True(t, found, "the verified domain is listed")
	ses := externalEnvelope(t, verified.Envelopes, fixtures.IntegrationAcmeSesID)
	smtp := externalEnvelope(t, verified.Envelopes, fixtures.IntegrationAcmeDefaultID)
	assert.Equal(t, externalapi.SendingDomainEnvelopeStateCustom, ses.State)
	assert.Equal(t, "bounce.mail.acme.com", ses.MailFrom.Value.Domain)
	assert.Equal(t, externalapi.MailFromStatusSuccess, ses.MailFrom.Value.Status)
	assert.Equal(t, externalapi.MxFailureBehaviorFallBack, ses.MailFrom.Value.OnMxFailure)
	assert.Equal(t, externalapi.SendingDomainEnvelopeStateSetAtProvider, smtp.State)
}
