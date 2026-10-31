package external_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/integration"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestExternalIntegrationsListReportsProviderLimitAndUsage(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	denied, err := env.ExternalScoped(t, "contacts:read").IntegrationsList(ctx, externalapi.IntegrationsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsListForbidden{}, denied, "integrations:read is required")

	env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(14).SetMaxPerDay(50000).ExecX(ctx)

	res, err := env.ExternalScoped(t, "integrations:read").IntegrationsList(ctx, externalapi.IntegrationsListParams{})
	require.NoError(t, err)
	list, ok := res.(*externalapi.IntegrationsListOK)
	require.Truef(t, ok, "got %T", res)
	require.GreaterOrEqual(t, len(list.Items), 2, "Acme's Integrations, ascending by id")
	for _, it := range list.Items {
		assert.NotEqual(t, fixtures.IntegrationGlobexName, it.Name, "Globex's Integrations are not visible")
	}

	def := list.Items[0]
	assert.Equal(t, fixtures.IntegrationAcmeDefaultName, def.Name)
	assert.True(t, def.IsDefault)
	assert.Equal(t, int32(14), def.ProviderLimit.PerSecond.Limit.Or(0))
	assert.Equal(t, externalapi.ProviderLimitSourceManual, def.ProviderLimit.PerSecond.Source.Or(""))
	assert.Equal(t, int32(50000), def.ProviderLimit.PerDay.Limit.Or(0))
	assert.Equal(t, int32(2), def.ProviderLimit.SentLast24h, "two sends in the last 24 hours; the third is older")
	assert.Empty(t, def.ProviderLimit.Warnings)

	ses := list.Items[1]
	assert.True(t, ses.ProviderLimit.PerSecond.Limit.Null)
	assert.True(t, ses.ProviderLimit.PerSecond.Source.Null)
	assert.Equal(t, []externalapi.ProviderLimitWarning{externalapi.ProviderLimitWarningUnlimited}, ses.ProviderLimit.Warnings)
	assert.Equal(t, int32(1), ses.ProviderLimit.SentLast24h)
}

func TestExternalIntegrationsListIsPaginatedAndWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "integrations:read")

	res, err := c.IntegrationsList(ctx, externalapi.IntegrationsListParams{PageSize: externalapi.NewOptInt32(1)})
	require.NoError(t, err)
	list := res.(*externalapi.IntegrationsListOK)
	assert.Len(t, list.Items, 1)
	total, err := env.DB.Scoped(fixtures.AcmeID).Integration().Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, narrow.Int32(total), list.TotalItems)
	assert.Equal(t, narrow.Int32(total), list.TotalPages, "one Integration a page")

	all, err := c.IntegrationsList(ctx, externalapi.IntegrationsListParams{})
	require.NoError(t, err)
	n, err := env.DB.Scoped(fixtures.AcmeID).Integration().Query().Where(integration.ProviderIn(integration.ProviderSMTP, integration.ProviderSes)).Count(ctx)
	require.NoError(t, err)
	assert.Len(t, all.(*externalapi.IntegrationsListOK).Items, n)
	for _, it := range all.(*externalapi.IntegrationsListOK).Items {
		assert.NotEqual(t, fixtures.IntegrationGlobexName, it.Name)
	}
}

// The resource carries the provider config redacted: the serialized response names no
// credential field and holds nothing of the stored secrets.
func TestExternalIntegrationsListRedactsSecrets(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := env.ExternalScoped(t, "integrations:read").IntegrationsList(ctx, externalapi.IntegrationsListParams{})
	require.NoError(t, err)
	list, ok := res.(*externalapi.IntegrationsListOK)
	require.Truef(t, ok, "got %T", res)
	body, err := json.Marshal(list)
	require.NoError(t, err)

	var decoded struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal(body, &decoded))
	require.NotEmpty(t, decoded.Items)
	for _, item := range decoded.Items {
		for key := range item {
			assert.NotContains(t, []string{"configEncrypted", "secret", "password", "secretAccessKey"}, key)
		}
	}
	for _, row := range env.DB.Integration.Query().AllX(ctx) {
		assert.NotContains(t, string(body), row.ConfigEncrypted, "the sealed config is not echoed")
	}
	for _, needle := range []string{"AKIAEXAMPLE", "secretAccessKey", "password"} {
		assert.NotContains(t, string(body), needle, "credentials never appear")
	}
}

// An SES-compatible service needs both Provider limit values on /api too (ADR 0067).
func TestExternalIntegrationsSesCompatibleRequiresTheProviderLimit(t *testing.T) {
	env := testhelper.Setup(t)
	res, err := env.ExternalScoped(t, "integrations:write").IntegrationsCreate(context.Background(), &externalapi.CreateIntegrationInput{
		Name: "Postbox",
		Params: externalapi.NewOptJsonObject(externalapi.JsonObject{
			"topic_arn": []byte(`"arn:aws:sns:eu-west-1:123456789012:acme-ses-events"`),
		}),
		MaxPerSecond: externalapi.NewOptNilMaxPerSecond(1),
		Config: externalapi.IntegrationConfigInput{
			OneOf: externalapi.NewSesConfigInputIntegrationConfigInputSum(externalapi.SesConfigInput{
				Kind: externalapi.SesConfigInputKindSes, Region: "ru-central1", AccessKeyId: "AKIA1234",
				SecretAccessKey: "secret", From: "noreply@acme.test",
				Endpoint: externalapi.NewOptNilString("https://postbox.cloud.yandex.net"),
			}),
		},
	})
	require.NoError(t, err)
	problem, ok := res.(*externalapi.IntegrationsCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, problem.Errors.Value, "maxPerDay")
	assert.NotContains(t, problem.Errors.Value, "maxPerSecond")
}
