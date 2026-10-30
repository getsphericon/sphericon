package site_test

import (
	"context"
	"testing"

	"github.com/go-faster/jx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/integration"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func smtpInput(host, password string) siteapi.SiteIntegrationConfigInput {
	return siteapi.SiteIntegrationConfigInput{
		OneOf: siteapi.NewSiteSmtpConfigInputSiteIntegrationConfigInputSum(siteapi.SiteSmtpConfigInput{
			Kind:     siteapi.SiteSmtpConfigInputKindSMTP,
			Host:     host,
			Port:     587,
			Username: siteapi.NewOptNilString("smtp-user"),
			Password: siteapi.NewOptNilString(password),
			From:     "noreply@acme.test",
		}),
	}
}

// sesTopicParams names the SNS topic an SES Integration needs to be saved (the `ses`
// Preset requires one).
var sesTopicParams = siteapi.NewOptSiteCreateIntegrationInputParams(siteapi.SiteCreateIntegrationInputParams{
	"topic_arn": jx.Raw(`"arn:aws:sns:eu-west-1:123456789012:acme-ses-events"`),
})

func sesInput(region, accessKey, secret string) siteapi.SiteIntegrationConfigInput {
	return siteapi.SiteIntegrationConfigInput{
		OneOf: siteapi.NewSiteSesConfigInputSiteIntegrationConfigInputSum(siteapi.SiteSesConfigInput{
			Kind:            siteapi.SiteSesConfigInputKindSes,
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

	// SES integration targeting an SES-compatible endpoint (e.g. Yandex Postbox).
	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Params: sesTopicParams,
		Name:   "Postbox",
		Config: siteapi.SiteIntegrationConfigInput{
			OneOf: siteapi.NewSiteSesConfigInputSiteIntegrationConfigInputSum(siteapi.SiteSesConfigInput{
				Kind:            siteapi.SiteSesConfigInputKindSes,
				Region:          "ru-central1",
				AccessKeyId:     "AKIA1234",
				SecretAccessKey: "secret",
				From:            "noreply@acme.test",
				Endpoint:        siteapi.NewOptNilString("https://postbox.cloud.yandex.net"),
			}),
		},
	}, params)
	require.NoError(t, err)
	res := created.(*siteapi.SiteIntegrationResource)

	// The endpoint is not a secret, so it round-trips on read.
	sesOut, ok := res.Config.OneOf.GetSiteSesConfig()
	require.Truef(t, ok, "config is ses")
	assert.Equal(t, "https://postbox.cloud.yandex.net", sesOut.Endpoint.Or(""))

	// An SES integration without an endpoint reads back with the endpoint unset.
	plain, err := c.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Params: sesTopicParams,
		Name:   "AWS",
		Config: sesInput("eu-west-1", "AKIA5678", "secret"),
	}, params)
	require.NoError(t, err)
	plainSes, ok := plain.(*siteapi.SiteIntegrationResource).Config.OneOf.GetSiteSesConfig()
	require.Truef(t, ok, "config is ses")
	assert.Empty(t, plainSes.Endpoint.Or(""))
}

func TestSiteIntegrationsCRUD(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID}

	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Name:   "Primary SMTP",
		Config: smtpInput("smtp.example.com", "s3cret-pass"),
	}, params)
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", created)
	assert.Equal(t, siteapi.SiteIntegrationProviderSMTP, res.Provider)
	assert.Equal(t, siteapi.SiteIntegrationChannelEmail, res.Channel)
	assert.True(t, res.Enabled)

	// The returned config is the redacted SMTP variant: host is present, and the
	// type itself has no password field, so the secret cannot leak.
	smtpOut, ok := res.Config.OneOf.GetSiteSmtpConfig()
	require.Truef(t, ok, "config is smtp")
	assert.Equal(t, "smtp.example.com", smtpOut.Host)
	assert.Equal(t, "smtp-user", smtpOut.Username.Or(""))

	// Stored config must be ciphertext, never the plaintext password.
	rowID := int64(res.ID)
	row, err := env.DB.Integration.Get(ctx, rowID)
	require.NoError(t, err)
	assert.NotEmpty(t, row.ConfigEncrypted)
	assert.NotContains(t, row.ConfigEncrypted, "s3cret-pass", "password is not stored in cleartext")
	assert.NotContains(t, row.ConfigEncrypted, "smtp.example.com", "config is encrypted, not cleartext")

	// Fetch the created integration back by id (selection by key).
	got, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: res.ID})
	require.NoError(t, err)
	gotRes, ok := got.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", got)
	assert.Equal(t, res.ID, gotRes.ID)

	// Update name + re-supply credentials.
	updated, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		Name:   siteapi.NewOptString("Renamed SMTP"),
		Config: siteapi.NewOptNilSiteIntegrationConfigInput(smtpInput("smtp2.example.com", "new-pass")),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: res.ID})
	require.NoError(t, err)
	updRes := updated.(*siteapi.SiteIntegrationResource)
	assert.Equal(t, "Renamed SMTP", updRes.Name)

	// There is no delete: archiving takes it out of the default list (ADR 0049).
	archived := archiveIntegration(t, c, int64(res.ID))
	assert.IsType(t, &siteapi.SiteIntegrationResource{}, archived)
	assert.NotContains(t, integrationIDs(listIntegrations(t, c, false)), int64(res.ID))
}

func TestSiteIntegrationsRejectsWrongProviderOnUpdate(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Name:   "SMTP",
		Config: smtpInput("smtp.example.com", "pw"),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	res := created.(*siteapi.SiteIntegrationResource)

	// Swapping the config to a different provider kind is rejected.
	got, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		Config: siteapi.NewOptNilSiteIntegrationConfigInput(sesInput("eu-west-1", "AKIA", "secret")),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteIntegrationsUpdateUnprocessableEntity{}, got)
}

func TestSiteIntegrationsSingleDefaultPerChannel(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID}

	first, err := c.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Name:      "First",
		IsDefault: siteapi.NewOptBool(true),
		Config:    smtpInput("smtp1.example.com", "pw"),
	}, params)
	require.NoError(t, err)
	firstID := first.(*siteapi.SiteIntegrationResource).ID

	second, err := c.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Params:    sesTopicParams,
		Name:      "Second",
		IsDefault: siteapi.NewOptBool(true),
		Config:    sesInput("eu-west-1", "AKIA1234", "secret"),
	}, params)
	require.NoError(t, err)
	secondRes := second.(*siteapi.SiteIntegrationResource)
	assert.True(t, secondRes.IsDefault)

	// Promoting the second default must have demoted the first.
	gotFirst, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: firstID})
	require.NoError(t, err)
	assert.False(t, gotFirst.(*siteapi.SiteIntegrationResource).IsDefault, "only one default per channel")

	// Exactly one row is default for the email channel.
	n, err := env.DB.Scoped(fixtures.AcmeID).Integration().Query().
		Where(integration.ChannelEQ(integration.ChannelEmail), integration.IsDefault(true)).
		Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestSiteIntegrationsPromoteViaUpdate(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID}

	first, err := c.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Name:      "First",
		IsDefault: siteapi.NewOptBool(true),
		Config:    smtpInput("smtp1.example.com", "pw"),
	}, params)
	require.NoError(t, err)
	firstID := first.(*siteapi.SiteIntegrationResource).ID

	second, err := c.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Params: sesTopicParams,
		Name:   "Second",
		Config: sesInput("eu-west-1", "AKIA1234", "secret"),
	}, params)
	require.NoError(t, err)
	secondID := second.(*siteapi.SiteIntegrationResource).ID

	// Promote the second to default via update; this must demote the first in the
	// same transaction (see promote path in SiteIntegrationsUpdate).
	updated, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		IsDefault: siteapi.NewOptBool(true),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: secondID})
	require.NoError(t, err)
	assert.True(t, updated.(*siteapi.SiteIntegrationResource).IsDefault)

	gotFirst, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: firstID})
	require.NoError(t, err)
	assert.False(t, gotFirst.(*siteapi.SiteIntegrationResource).IsDefault, "promoting the second demotes the first")

	n, err := env.DB.Scoped(fixtures.AcmeID).Integration().Query().
		Where(integration.ChannelEQ(integration.ChannelEmail), integration.IsDefault(true)).
		Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestSiteIntegrationsScopedToWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteIntegrationsList(context.Background(), siteapi.SiteIntegrationsListParams{WorkspaceId: "NoSuchWs"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, res)
}

func createSMTP(t *testing.T, c *siteapi.Client, host, password string) siteapi.EntityId {
	t.Helper()
	created, err := c.SiteIntegrationsCreate(context.Background(), &siteapi.SiteCreateIntegrationInput{
		Name:   "Primary SMTP",
		Config: smtpInput(host, password),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	return created.(*siteapi.SiteIntegrationResource).ID
}

func TestSiteIntegrationsWritesNeedAnOwnerOrAdmin(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	id := createSMTP(t, env.SiteActor(t, fixtures.OwnerJohnEmail), "smtp.example.com", "s3cret-pass")
	member := env.SiteActor(t, fixtures.MemberMaryEmail)

	created, err := member.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Name: "Mine", Config: smtpInput("attacker.example", "pw"),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteIntegrationsCreateForbidden), created)

	updated, err := member.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		Config: siteapi.NewOptNilSiteIntegrationConfigInput(smtpInput("attacker.example", "")),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteIntegrationsUpdateForbidden), updated)

	deleted, err := member.SiteIntegrationsArchive(ctx, siteapi.SiteIntegrationsArchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteIntegrationsArchiveForbidden), deleted)

	// Reading stays open to a member, secrets redacted.
	got, err := member.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteIntegrationResource), got)

	// An admin manages them.
	ok, err := env.SiteActor(t, fixtures.AdminAliceEmail).SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		Name: siteapi.NewOptString("Renamed"),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteIntegrationResource), ok)
}

func TestSiteIntegrationsBlankSecretIsKeptOnlyForTheSameDestination(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := createSMTP(t, c, "smtp.example.com", "s3cret-pass")
	params := siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: id}

	// Same host, port and username: the blank password keeps the stored one.
	kept, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		Config: siteapi.NewOptNilSiteIntegrationConfigInput(smtpInput("smtp.example.com", "")),
	}, params)
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteIntegrationResource), kept)

	// A new host with a blank password is refused rather than sent the stored secret.
	moved, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		Config: siteapi.NewOptNilSiteIntegrationConfigInput(smtpInput("attacker.example", "")),
	}, params)
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteIntegrationsUpdateUnprocessableEntity), moved)

	// Supplying the password again is how a destination changes.
	ok, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		Config: siteapi.NewOptNilSiteIntegrationConfigInput(smtpInput("smtp2.example.com", "new-pass")),
	}, params)
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteIntegrationResource), ok)
}

func TestSiteIntegrationsSesBlankSecretFollowsRegionAndKey(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Params: sesTopicParams,
		Name:   "AWS", Config: sesInput("eu-west-1", "AKIA1234", "secret"),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	params := siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: created.(*siteapi.SiteIntegrationResource).ID}

	kept, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		Config: siteapi.NewOptNilSiteIntegrationConfigInput(sesInput("eu-west-1", "AKIA1234", "")),
	}, params)
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteIntegrationResource), kept)

	for name, cfg := range map[string]siteapi.SiteIntegrationConfigInput{
		"new key id": sesInput("eu-west-1", "AKIA9999", ""),
		"new region": sesInput("us-east-1", "AKIA1234", ""),
	} {
		res, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
			Config: siteapi.NewOptNilSiteIntegrationConfigInput(cfg),
		}, params)
		require.NoError(t, err, name)
		assert.IsType(t, new(siteapi.SiteIntegrationsUpdateUnprocessableEntity), res, name)
	}
}

func TestSiteIntegrationsRefuseAnInsecureSesEndpoint(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	res, err := c.SiteIntegrationsCreate(context.Background(), &siteapi.SiteCreateIntegrationInput{
		Params: sesTopicParams,
		Name:   "Bad",
		Config: siteapi.SiteIntegrationConfigInput{
			OneOf: siteapi.NewSiteSesConfigInputSiteIntegrationConfigInputSum(siteapi.SiteSesConfigInput{
				Kind: siteapi.SiteSesConfigInputKindSes, Region: "r", AccessKeyId: "AKIA1", SecretAccessKey: "s",
				From: "noreply@acme.test", Endpoint: siteapi.NewOptNilString("http://169.254.169.254"),
			}),
		},
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteIntegrationsCreateUnprocessableEntity), res)
}
