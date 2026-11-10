package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-faster/jx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/integration"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func smtpInput(host, password string) siteapi.IntegrationConfigInput {
	return siteapi.IntegrationConfigInput{
		OneOf: siteapi.NewSmtpConfigInputIntegrationConfigInputSum(siteapi.SmtpConfigInput{
			Kind:     siteapi.SmtpConfigInputKindSMTP,
			Host:     host,
			Port:     587,
			Username: siteapi.NewOptNilString("mailer"),
			Password: siteapi.NewOptNilString(password),
			From:     "noreply@acme.test",
		}),
	}
}

// sesTopicParams names the SNS topic an SES Integration needs to be saved (the `ses`
// Preset requires one).
var sesTopicParams = siteapi.NewOptJsonObject(siteapi.JsonObject{
	"topic_arn": jx.Raw(`"arn:aws:sns:eu-west-1:123456789012:acme-ses-events"`),
})

func sesInput(region, accessKey, secret string) siteapi.IntegrationConfigInput {
	return siteapi.IntegrationConfigInput{
		OneOf: siteapi.NewSesConfigInputIntegrationConfigInputSum(siteapi.SesConfigInput{
			Kind:            siteapi.SesConfigInputKindSes,
			Region:          region,
			AccessKeyId:     accessKey,
			SecretAccessKey: secret,
			From:            "noreply@acme.test",
		}),
	}
}

func TestSiteIntegrationsSesEndpointRoundTrips(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID}

	// SES integration targeting an SES-compatible endpoint (e.g. Yandex Postbox), which
	// takes its Provider limit.
	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{
		Params:       sesTopicParams,
		Name:         "Postbox",
		MaxPerSecond: siteapi.NewOptNilMaxPerSecond(1),
		MaxPerDay:    siteapi.NewOptNilMaxPerDay(200),
		Config: siteapi.IntegrationConfigInput{
			OneOf: siteapi.NewSesConfigInputIntegrationConfigInputSum(siteapi.SesConfigInput{
				Kind:            siteapi.SesConfigInputKindSes,
				Region:          "ru-central1",
				AccessKeyId:     "AKIA1234",
				SecretAccessKey: "secret",
				From:            "noreply@acme.test",
				Endpoint:        siteapi.NewOptNilString("https://postbox.cloud.yandex.net"),
			}),
		},
	}, params)
	require.NoError(t, err)
	res := created

	// The endpoint is not a secret, so it round-trips on read.
	sesOut, ok := res.Config.OneOf.GetSiteSesConfig()
	require.Truef(t, ok, "config is ses")
	assert.Equal(t, "https://postbox.cloud.yandex.net", sesOut.Endpoint.Or(""))

	// An SES integration without an endpoint reads back with the endpoint unset.
	plain, err := c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{
		Params: sesTopicParams,
		Name:   "AWS",
		Config: sesInput("eu-west-1", "AKIA5678", "secret"),
	}, params)
	require.NoError(t, err)
	plainSes, ok := plain.Config.OneOf.GetSiteSesConfig()
	require.Truef(t, ok, "config is ses")
	assert.Empty(t, plainSes.Endpoint.Or(""))
}

func TestSiteIntegrationsCRUD(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID}

	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{
		Name:   "Primary SMTP",
		Config: smtpInput("smtp.example.com", "s3cret-pass"),
	}, params)
	require.NoError(t, err)
	res := created
	assert.Equal(t, siteapi.SiteIntegrationProviderSMTP, res.Provider)
	assert.Equal(t, siteapi.SiteIntegrationChannelEmail, res.Channel)
	assert.True(t, res.Enabled)

	// The returned config is the redacted SMTP variant: host is present, and the
	// type itself has no password field, so the secret cannot leak.
	smtpOut, ok := res.Config.OneOf.GetSiteSmtpConfig()
	require.Truef(t, ok, "config is smtp")
	assert.Equal(t, "smtp.example.com", smtpOut.Host)
	assert.Equal(t, "mailer", smtpOut.Username.Or(""))

	// Stored config must be ciphertext, never the plaintext password.
	rowID := int64(res.ID)
	row, err := env.DB.Integration.Get(ctx, rowID)
	require.NoError(t, err)
	assert.NotEmpty(t, row.ConfigEncrypted)
	assert.NotContains(t, row.ConfigEncrypted, "s3cret-pass", "password is not stored in cleartext")
	assert.NotContains(t, row.ConfigEncrypted, "smtp.example.com", "config is encrypted, not cleartext")

	// Fetch the created integration back by id (selection by key).
	gotRes, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: res.ID})
	require.NoError(t, err)
	assert.Equal(t, res.ID, gotRes.ID)

	// Update name + re-supply credentials.
	updated, err := c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
		Name:   siteapi.NewOptName("Renamed SMTP"),
		Config: siteapi.NewOptIntegrationConfigInput(smtpInput("smtp2.example.com", "new-pass")),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: res.ID})
	require.NoError(t, err)
	updRes := updated
	assert.Equal(t, "Renamed SMTP", updRes.Name)

	// Delete takes it out of the list for good; the row is discarded, not removed (ADR 0049).
	require.NoError(t, deleteIntegration(t, c, int64(res.ID)))
	assert.NotContains(t, integrationIDs(listIntegrations(t, c)), int64(res.ID))
}

func TestSiteIntegrationsRejectsWrongProviderOnUpdate(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	// Swapping an SMTP relay's config to a different provider kind is rejected.
	_, err := c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
		Config: siteapi.NewOptIntegrationConfigInput(sesInput("eu-west-1", "AKIA", "secret")),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.IntegrationAcmeGenericAID})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}

func TestSiteIntegrationsSingleDefaultPerChannel(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID}

	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{
		Params:    sesTopicParams,
		Name:      "New default",
		IsDefault: siteapi.NewOptBool(true),
		Config:    sesInput("eu-west-1", "AKIA1234", "secret"),
	}, params)
	require.NoError(t, err)
	assert.True(t, created.IsDefault)

	// Creating a new default must have demoted the old one.
	gotFirst, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.IntegrationAcmeDefaultID})
	require.NoError(t, err)
	assert.False(t, gotFirst.IsDefault, "only one default per channel")

	// Exactly one row is default for the email channel.
	n, err := env.DB.Scoped(fixtures.AcmeID).Integration().Query().
		Where(integration.ChannelEQ(channels.Email), integration.IsDefault(true)).
		Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestSiteIntegrationsPromoteViaUpdate(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	// Promote a relay to default via update; this must demote the old default in the
	// same transaction (see promote path in SiteIntegrationsUpdate).
	updated, err := c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
		IsDefault: siteapi.NewOptBool(true),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.IntegrationAcmeGenericAID})
	require.NoError(t, err)
	assert.True(t, updated.IsDefault)

	gotFirst, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.IntegrationAcmeDefaultID})
	require.NoError(t, err)
	assert.False(t, gotFirst.IsDefault, "promoting a relay demotes the old default")

	n, err := env.DB.Scoped(fixtures.AcmeID).Integration().Query().
		Where(integration.ChannelEQ(channels.Email), integration.IsDefault(true)).
		Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestSiteIntegrationsScopedToWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteIntegrationsList(context.Background(), siteapi.SiteIntegrationsListParams{WorkspaceId: "NoSuchWs"})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

func TestSiteIntegrationsWritesNeedAnOwnerOrAdmin(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	id := siteapi.EntityId(fixtures.IntegrationAcmeSmtpLoginID)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)

	_, err := member.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{
		Name: "Mine", Config: smtpInput("attacker.example", "pw"),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	_, err = member.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
		Config: siteapi.NewOptIntegrationConfigInput(smtpInput("attacker.example", "")),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	err = member.SiteIntegrationsDelete(ctx, siteapi.SiteIntegrationsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)

	// Reading stays open to a member, secrets redacted.
	_, err = member.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)

	// An admin manages them.
	_, err = env.SiteActor(t, fixtures.AdminAliceEmail).SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
		Name: siteapi.NewOptName("Renamed"),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
}

func TestSiteIntegrationsBlankSecretIsKeptOnlyForTheSameDestination(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	// The relay logs in to smtp.example.com:587 as mailer with a stored password.
	params := siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.IntegrationAcmeSmtpLoginID}

	// Same host, port and username: the blank password keeps the stored one.
	_, err := c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
		Config: siteapi.NewOptIntegrationConfigInput(smtpInput("smtp.example.com", "")),
	}, params)
	require.NoError(t, err)

	// A new host with a blank password is refused rather than sent the stored secret.
	_, err = c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
		Config: siteapi.NewOptIntegrationConfigInput(smtpInput("attacker.example", "")),
	}, params)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	// Supplying the password again is how a destination changes.
	_, err = c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
		Config: siteapi.NewOptIntegrationConfigInput(smtpInput("smtp2.example.com", "new-pass")),
	}, params)
	require.NoError(t, err)
}

func TestSiteIntegrationsSesBlankSecretFollowsRegionAndKey(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{
		Params: sesTopicParams,
		Name:   "AWS", Config: sesInput("eu-west-1", "AKIA1234", "secret"),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	params := siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID}

	_, err = c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
		Config: siteapi.NewOptIntegrationConfigInput(sesInput("eu-west-1", "AKIA1234", "")),
	}, params)
	require.NoError(t, err)

	for name, cfg := range map[string]siteapi.IntegrationConfigInput{
		"new key id": sesInput("eu-west-1", "AKIA9999", ""),
		"new region": sesInput("us-east-1", "AKIA1234", ""),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
				Config: siteapi.NewOptIntegrationConfigInput(cfg),
			}, params)
			testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
		})
	}
}

func TestSiteIntegrationsRefuseAnInsecureSesEndpoint(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	_, err := c.SiteIntegrationsCreate(context.Background(), &siteapi.CreateIntegrationInput{
		Params: sesTopicParams,
		Name:   "Bad",
		Config: siteapi.IntegrationConfigInput{
			OneOf: siteapi.NewSesConfigInputIntegrationConfigInputSum(siteapi.SesConfigInput{
				Kind: siteapi.SesConfigInputKindSes, Region: "r", AccessKeyId: "AKIA1", SecretAccessKey: "s",
				From: "noreply@acme.test", Endpoint: siteapi.NewOptNilString("http://169.254.169.254"),
			}),
		},
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}
