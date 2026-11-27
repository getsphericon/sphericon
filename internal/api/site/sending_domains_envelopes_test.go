package site_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Sending domain shows the envelope of each email Integration of the Workspace, where SPF is
// checked, instead of an SPF record of its own (ADR 0184). The SES envelope is what the domain's
// check read of its identity at SES.

// verifyNews runs the check of news.acme.com through /site and returns the domain as it left it.
func verifyNews(t *testing.T, env *testhelper.TestEnv) *siteapi.SendingDomainResource {
	t.Helper()
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteSendingDomainsVerify(t.Context(), siteapi.SiteSendingDomainsVerifyParams{
		WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.SendingDomainUnverifiedID),
	})
	require.NoError(t, err)
	return res
}

// envelopeOf is the envelope of the Integration id in res; the test fails when there is none.
func envelopeOf(t *testing.T, res *siteapi.SendingDomainResource, id int64) siteapi.SendingDomainEnvelope {
	t.Helper()
	for _, e := range res.Envelopes {
		if e.IntegrationId == siteapi.EntityId(id) {
			return e
		}
	}
	require.Failf(t, "no envelope", "integration %d has no envelope", id)
	return siteapi.SendingDomainEnvelope{}
}

func TestSiteSendingDomainShowsACustomMailFromOfSES(t *testing.T) {
	env := testhelper.Setup(t)
	env.SES.SetMailFrom(fixtures.SendingDomainUnverifiedDomain, messaging.MailFrom{
		Domain: "bounce.news.acme.com", Status: providers.MailFromFailed, OnMXFailure: providers.MXFailureReject,
	})

	res := verifyNews(t, env)

	ses := envelopeOf(t, res, fixtures.IntegrationAcmeSesID)
	assert.Equal(t, siteapi.SendingDomainEnvelopeStateCustom, ses.State)
	assert.Equal(t, siteapi.IntegrationProviderSes, ses.Provider)
	require.False(t, ses.MailFrom.Null, "a custom envelope carries its MAIL FROM")
	assert.Equal(t, "bounce.news.acme.com", ses.MailFrom.Value.Domain)
	assert.Equal(t, siteapi.MailFromStatusFailed, ses.MailFrom.Value.Status)
	assert.Equal(t, siteapi.MxFailureBehaviorReject, ses.MailFrom.Value.OnMxFailure)
	assert.True(t, ses.PermissionNeeded.Null)
	assert.False(t, ses.CheckedAt.Null, "the check read it")
}

func TestSiteSendingDomainShowsSESOwnMailFromWithoutACustomOne(t *testing.T) {
	env := testhelper.Setup(t)

	res := verifyNews(t, env)

	ses := envelopeOf(t, res, fixtures.IntegrationAcmeSesID)
	assert.Equal(t, siteapi.SendingDomainEnvelopeStateProviderDefault, ses.State)
	assert.True(t, ses.MailFrom.Null)
	assert.True(t, ses.PermissionNeeded.Null)
}

func TestSiteSendingDomainShowsAnEnvelopeSESCredentialsCannotReadAsUnknown(t *testing.T) {
	env := testhelper.Setup(t)
	env.SES.SetMailFromErr(errors.New("AccessDeniedException: not authorized to perform ses:GetEmailIdentity"))

	res := verifyNews(t, env)

	ses := envelopeOf(t, res, fixtures.IntegrationAcmeSesID)
	assert.Equal(t, siteapi.SendingDomainEnvelopeStateUnknown, ses.State)
	assert.Equal(t, "ses:GetEmailIdentity", ses.PermissionNeeded.Value, "the permission it needs is named")
	assert.True(t, ses.MailFrom.Null)
}

func TestSiteSendingDomainShowsAnSMTPRelayAsSetAtItsProviderAndNoTestInbox(t *testing.T) {
	env := testhelper.Setup(t)

	res := verifyNews(t, env)

	smtp := envelopeOf(t, res, fixtures.IntegrationAcmeDefaultID)
	assert.Equal(t, siteapi.SendingDomainEnvelopeStateSetAtProvider, smtp.State)
	assert.Equal(t, siteapi.IntegrationProviderSMTP, smtp.Provider)
	assert.True(t, smtp.MailFrom.Null, "no record for an SMTP relay")
	assert.True(t, smtp.CheckedAt.Null, "an SMTP relay is asked nothing")
	for _, e := range res.Envelopes {
		assert.NotEqual(t, siteapi.IntegrationProviderInbox, e.Provider, "the Test inbox has no envelope")
	}
}

func TestSiteSendingDomainShowsOneEnvelopePerEmailIntegration(t *testing.T) {
	env := testhelper.Setup(t)

	res := verifyNews(t, env)

	require.NotEmpty(t, res.Envelopes)
	ids := map[siteapi.EntityId]bool{}
	for _, e := range res.Envelopes {
		assert.Falsef(t, ids[e.IntegrationId], "integration %d has one envelope", e.IntegrationId)
		ids[e.IntegrationId] = true
	}
	assert.True(t, ids[siteapi.EntityId(fixtures.IntegrationAcmeSesID)])
	assert.True(t, ids[siteapi.EntityId(fixtures.IntegrationAcmeDefaultID)])
	assert.False(t, ids[siteapi.EntityId(fixtures.IntegrationAcmeArchivedID)], "a deleted Integration has none")
}

func TestSiteSendingDomainHasNoSPFRecordOfItsOwn(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteSendingDomainsGet(t.Context(), siteapi.SiteSendingDomainsGetParams{
		WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.SendingDomainVerifiedID),
	})
	require.NoError(t, err)

	raw, err := json.Marshal(res)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.NotContains(t, fields, "spfRecord")
	assert.NotContains(t, string(raw), "v=spf1")
}
