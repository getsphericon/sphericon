package integrations_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Integration is read with its config and params without a secret, the hook URL its provider
// posts to and its Provider limit as enforced: what every surface shows of it.
func TestViewShowsTheIntegrationWithoutASecret(t *testing.T) {
	env := testhelper.Setup(t)

	v, err := env.Integrations.View(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.IntegrationAcmeSesID)

	require.NoError(t, err)
	assert.Equal(t, int64(fixtures.IntegrationAcmeSesID), v.Row.ID)
	assert.Equal(t, &integrations.RedactedSES{
		Region: "eu-west-1", From: "hello@codebasics.dev", AccessKeyIDLast4: "MPLE",
	}, v.Config.SES, "the access key id shows its last four characters, the secret key nothing")
	assert.Equal(t, env.AppURL+"/hooks/omik_test_integrationacmeses", v.HookURL)
	assert.JSONEq(t, `"arn:aws:sns:eu-west-1:123456789012:acme-ses-events"`, string(v.Params["topic_arn"]))
	assert.Empty(t, v.SecretsRetiring)
	assert.Equal(t, providerlimit.Status{Sent24h: 1, Warnings: []providerlimit.Warning{providerlimit.WarningUnlimited}}, v.ProviderLimit,
		"the SES Integration sent one message over the last 24 hours and has no ceiling")
}

func TestViewOfAnotherWorkspacesIntegrationIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.Integrations.View(t.Context(), env.DB.Scoped(fixtures.GlobexID), fixtures.IntegrationAcmeSesID)

	require.ErrorIs(t, err, integrations.ErrNotFound)
}

// The list reads each Integration's view, the 24-hour usage once for all of them.
func TestListAndAllShowEveryIntegrationAsItsView(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)

	all, err := env.Integrations.All(t.Context(), s)
	require.NoError(t, err)
	page, err := env.Integrations.List(t.Context(), s, pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, err)

	byID := lo.KeyBy(all, func(v integrations.View) int64 { return v.Row.ID })
	require.Contains(t, byID, int64(fixtures.IntegrationAcmeDefaultID))
	smtp := byID[fixtures.IntegrationAcmeDefaultID]
	assert.Equal(t, "mailpit", smtp.Config.SMTP.Host)
	assert.Equal(t, env.AppURL+"/hooks/omik_test_integrationacmedefault", smtp.HookURL)
	assert.Equal(t, 2, smtp.ProviderLimit.Sent24h, "the usage of each Integration, read once for the list")
	assert.Equal(t, 1, byID[fixtures.IntegrationAcmeSesID].ProviderLimit.Sent24h)
	assert.Equal(t, ids(all), ids(page.Items), "both lists show the same Integrations")
	assert.Equal(t, hooks(all), hooks(page.Items))
}

func ids(views []integrations.View) []int64 {
	return lo.Map(views, func(v integrations.View, _ int) int64 { return v.Row.ID })
}

func hooks(views []integrations.View) []string {
	return lo.Map(views, func(v integrations.View, _ int) string { return v.HookURL })
}
