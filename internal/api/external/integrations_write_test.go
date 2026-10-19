package external_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/integration"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const smtpSecret = "hunter2-smtp-secret"

func smtpInput(host string) externalapi.IntegrationConfigInput {
	return externalapi.IntegrationConfigInput{OneOf: externalapi.NewSmtpConfigInputIntegrationConfigInputSum(externalapi.SmtpConfigInput{
		Kind:     externalapi.SmtpConfigInputKindSMTP,
		Host:     host,
		Port:     587,
		Username: externalapi.NewOptNilString("mailer"),
		Password: externalapi.NewOptNilString(smtpSecret),
		From:     "sender@acme.test",
	})}
}

func createSMTP(t *testing.T, c *externalapi.Client, name string, isDefault bool) *externalapi.IntegrationResource {
	t.Helper()
	res, err := c.IntegrationsCreate(context.Background(), &externalapi.CreateIntegrationInput{
		Name: name, IsDefault: externalapi.NewOptBool(isDefault), Config: smtpInput("smtp.example.com"),
	})
	require.NoError(t, err)
	created, ok := res.(*externalapi.IntegrationResource)
	require.Truef(t, ok, "got %T", res)
	return created
}

func TestExternalIntegrationsWritesNeedTheWriteScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	reader := env.ExternalScoped(t, "integrations:read")
	id := entityIDString(fixtures.IntegrationAcmeSesID)

	create, err := reader.IntegrationsCreate(ctx, &externalapi.CreateIntegrationInput{Name: "x", Config: smtpInput("smtp.example.com")})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsCreateUnauthorized{}, create)

	upd, err := reader.IntegrationsUpdate(ctx, &externalapi.UpdateIntegrationInput{Name: externalapi.NewOptString("x")}, externalapi.IntegrationsUpdateParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsUpdateUnauthorized{}, upd)

	del, err := reader.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsDeleteUnauthorized{}, del)

	writer := env.ExternalScoped(t, "integrations:write")
	get, err := writer.IntegrationsGet(ctx, externalapi.IntegrationsGetParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsGetUnauthorized{}, get, "write does not imply read")

	n, err := env.DB.Scoped(fixtures.AcmeID).Integration().Query().Where(integration.ID(fixtures.IntegrationAcmeSesID)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "a refused delete leaves the row")
}

func TestExternalIntegrationsCreateNeverReturnsSecrets(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")

	created := createSMTP(t, c, "Acme transactional", false)
	assert.Equal(t, externalapi.IntegrationProviderSMTP, created.Provider)
	assert.Equal(t, externalapi.IntegrationChannelEmail, created.Channel)
	assert.True(t, created.Enabled)
	assert.False(t, created.IsDefault)
	smtp, ok := created.Config.OneOf.GetSmtpConfig()
	require.True(t, ok)
	assert.Equal(t, "smtp.example.com", smtp.Host)
	assert.Equal(t, "mailer", smtp.Username.Or(""))

	row, err := env.DB.Scoped(fixtures.AcmeID).Integration().Get(ctx, int64(created.ID))
	require.NoError(t, err)
	assert.NotContains(t, row.ConfigEncrypted, smtpSecret, "the password is sealed at rest")

	get, err := c.IntegrationsGet(ctx, externalapi.IntegrationsGetParams{ID: created.ID})
	require.NoError(t, err)
	list, err := c.IntegrationsList(ctx, externalapi.IntegrationsListParams{})
	require.NoError(t, err)
	for name, v := range map[string]any{"create": created, "get": get, "list": list} {
		body, err := json.Marshal(v)
		require.NoError(t, err)
		assert.NotContains(t, string(body), smtpSecret, name)
		assert.NotContains(t, string(body), "password", name)
		assert.NotContains(t, string(body), row.ConfigEncrypted, name)
	}
}

func TestExternalIntegrationsCreateValidates(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "integrations:write")
	ctx := context.Background()

	blank, err := c.IntegrationsCreate(ctx, &externalapi.CreateIntegrationInput{Name: "  ", Config: smtpInput("smtp.example.com")})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsCreateUnprocessableEntity{}, blank)

	noHost, err := c.IntegrationsCreate(ctx, &externalapi.CreateIntegrationInput{Name: "no host", Config: smtpInput("")})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsCreateUnprocessableEntity{}, noHost, "the provider's own config check runs")
}

func TestExternalIntegrationsNewDefaultReplacesTheOldOne(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")

	created := createSMTP(t, c, "New default", true)
	assert.True(t, created.IsDefault)

	old, err := c.IntegrationsGet(ctx, externalapi.IntegrationsGetParams{ID: entityIDString(fixtures.IntegrationAcmeDefaultID)})
	require.NoError(t, err)
	assert.False(t, old.(*externalapi.IntegrationResource).IsDefault, "one default per channel")
}

func TestExternalIntegrationsUpdateKeepsCredentialsAndRedacts(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	created := createSMTP(t, c, "To edit", false)
	scoped := env.DB.Scoped(fixtures.AcmeID)
	before, err := scoped.Integration().Get(ctx, int64(created.ID))
	require.NoError(t, err)

	renamed, err := c.IntegrationsUpdate(ctx, &externalapi.UpdateIntegrationInput{
		Name: externalapi.NewOptString("Renamed"), Enabled: externalapi.NewOptBool(false),
		MaxPerSecond: externalapi.NewOptNilMaxPerSecond(externalapi.MaxPerSecond(5)),
	}, externalapi.IntegrationsUpdateParams{ID: created.ID})
	require.NoError(t, err)
	res, ok := renamed.(*externalapi.IntegrationResource)
	require.Truef(t, ok, "got %T", renamed)
	assert.Equal(t, "Renamed", res.Name)
	assert.False(t, res.Enabled)
	assert.Equal(t, int32(5), res.MaxPerSecond.Value)
	after, err := scoped.Integration().Get(ctx, before.ID)
	require.NoError(t, err)
	assert.Equal(t, before.ConfigEncrypted, after.ConfigEncrypted, "omitting config keeps the credentials")

	// A blank password inside a supplied config keeps the stored secret while the host,
	// port and username are unchanged; the response stays redacted either way.
	blankPassword := func(host string) externalapi.IntegrationConfigInput {
		smtpIn := smtpInput(host).OneOf.SmtpConfigInput
		smtpIn.Password = externalapi.OptNilString{}
		return externalapi.IntegrationConfigInput{OneOf: externalapi.NewSmtpConfigInputIntegrationConfigInputSum(smtpIn)}
	}
	cfg := blankPassword("smtp.example.com")
	edited, err := c.IntegrationsUpdate(ctx, &externalapi.UpdateIntegrationInput{
		Config:       externalapi.NewOptNilIntegrationConfigInput(cfg),
		MaxPerSecond: externalapi.OptNilMaxPerSecond{Null: true, Set: true},
	}, externalapi.IntegrationsUpdateParams{ID: created.ID})
	require.NoError(t, err)
	res, ok = edited.(*externalapi.IntegrationResource)
	require.Truef(t, ok, "got %T", edited)
	got, ok := res.Config.OneOf.GetSmtpConfig()
	require.True(t, ok)
	assert.Equal(t, "smtp.example.com", got.Host)
	assert.True(t, res.MaxPerSecond.Null, "null clears the limit")
	body, err := json.Marshal(res)
	require.NoError(t, err)
	assert.NotContains(t, string(body), smtpSecret)

	// Pointing the Integration at another host without the password is refused: the stored
	// secret is never sent to a destination it was not saved for.
	moved, err := c.IntegrationsUpdate(ctx, &externalapi.UpdateIntegrationInput{Config: externalapi.NewOptNilIntegrationConfigInput(blankPassword("attacker.example"))},
		externalapi.IntegrationsUpdateParams{ID: created.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsUpdateUnprocessableEntity{}, moved)

	// A config of the other provider kind is refused.
	sesCfg := externalapi.IntegrationConfigInput{OneOf: externalapi.NewSesConfigInputIntegrationConfigInputSum(externalapi.SesConfigInput{
		Kind: externalapi.SesConfigInputKindSes, Region: "eu-west-1", AccessKeyId: "AKIAXXXX", SecretAccessKey: "s", From: "a@acme.test",
	})}
	mismatch, err := c.IntegrationsUpdate(ctx, &externalapi.UpdateIntegrationInput{Config: externalapi.NewOptNilIntegrationConfigInput(sesCfg)},
		externalapi.IntegrationsUpdateParams{ID: created.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsUpdateUnprocessableEntity{}, mismatch)
}

func TestExternalIntegrationsDelete(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	created := createSMTP(t, c, "Short lived", false)

	del, err := c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: created.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsDeleteNoContent{}, del)

	get, err := c.IntegrationsGet(ctx, externalapi.IntegrationsGetParams{ID: created.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsGetNotFound{}, get)

	again, err := c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: created.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsDeleteNotFound{}, again)
}

// Another Workspace's Integration is invisible and untouchable through this token.
func TestExternalIntegrationsAreWorkspaceIsolated(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	globex := entityIDString(fixtures.IntegrationGlobexID)

	get, err := c.IntegrationsGet(ctx, externalapi.IntegrationsGetParams{ID: globex})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsGetNotFound{}, get)

	upd, err := c.IntegrationsUpdate(ctx, &externalapi.UpdateIntegrationInput{Name: externalapi.NewOptString("hijacked")}, externalapi.IntegrationsUpdateParams{ID: globex})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsUpdateNotFound{}, upd)

	del, err := c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: globex})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsDeleteNotFound{}, del)

	row, err := env.DB.Integration.Get(ctx, fixtures.IntegrationGlobexID)
	require.NoError(t, err)
	assert.Equal(t, fixtures.IntegrationGlobexName, row.Name)
}

// An SES Integration reads its callbacks with the `ses` Preset without being told so, and
// the SNS topic it accepts is a param of that Preset: returned on read, and refused when it
// is not a topic ARN.
func TestExternalIntegrationsSESNamesItsSNSTopicInTheParams(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	ses := func(topic string) *externalapi.CreateIntegrationInput {
		return &externalapi.CreateIntegrationInput{
			Name: "SES " + topic,
			Config: externalapi.IntegrationConfigInput{OneOf: externalapi.NewSesConfigInputIntegrationConfigInputSum(externalapi.SesConfigInput{
				Kind: externalapi.SesConfigInputKindSes, Region: "eu-west-1", AccessKeyId: "AKIAXXXX", SecretAccessKey: "s", From: "a@acme.test",
			})},
			Params: externalapi.NewOptCreateIntegrationInputParams(externalapi.CreateIntegrationInputParams{"topic_arn": jx.Raw(strconv.Quote(topic))}),
		}
	}

	bad, err := c.IntegrationsCreate(ctx, ses("not-an-arn"))
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsCreateUnprocessableEntity{}, bad)

	// An SES Integration without a topic could never receive a bounce: it is refused at save.
	noTopic := ses("unused")
	noTopic.Params = externalapi.OptCreateIntegrationInputParams{}
	missing, err := c.IntegrationsCreate(ctx, noTopic)
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsCreateUnprocessableEntity{}, missing)

	const topic = "arn:aws:sns:eu-west-1:123456789012:ses-events"
	res, err := c.IntegrationsCreate(ctx, ses(topic))
	require.NoError(t, err)
	created, ok := res.(*externalapi.IntegrationResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, "ses", created.Preset.Or(""))
	assert.JSONEq(t, strconv.Quote(topic), string(created.Params["topic_arn"]))
}
