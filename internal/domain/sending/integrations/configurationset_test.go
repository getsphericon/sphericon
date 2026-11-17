package integrations_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging/ses"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An SES Integration names a configuration set to route its mail to a dedicated pool. Saving it
// asks SES whether the set exists, best effort: an unknown one saves with a warning, and a check
// that gets no answer saves without one.

func createSESWithSet(t *testing.T, env *testhelper.TestEnv, set string) integrations.View {
	t.Helper()
	v, err := testhelper.Module[*integrations.Module](env).Create(t.Context(), env.DB.Scoped(fixtures.AcmeID), integrations.CreateInput{
		Name: "Marketing",
		Config: integrations.ConfigInput{SES: &integrations.SESConfig{
			Region: "eu-west-1", AccessKeyID: "AKIA1234", SecretAccessKey: "s", From: "news@acme.test", ConfigurationSet: set,
		}},
		Params: map[string]any{"topic_arn": "arn:aws:sns:eu-west-1:123456789012:acme-marketing"},
	})
	require.NoError(t, err)
	return v
}

func TestAnSESIntegrationKeepsItsConfigurationSet(t *testing.T) {
	env := testhelper.Setup(t)

	v := createSESWithSet(t, env, "marketing")

	got := stored[ses.Config](t, env, v.Row.ID)

	assert.Equal(t, "marketing", v.Config.SES.ConfigurationSet)
	assert.False(t, v.Config.SES.ConfigurationSetMissing)
	assert.Equal(t, "marketing", got.ConfigurationSet, "every send names it")
}

func TestAnUnknownConfigurationSetSavesWithAWarning(t *testing.T) {
	env := testhelper.Setup(t)
	env.SES.SetConfigurationSetMissing(true)

	v := createSESWithSet(t, env, "marketing")

	assert.True(t, v.Config.SES.ConfigurationSetMissing)
	got, err := testhelper.Module[*integrations.Module](env).View(t.Context(), env.DB.Scoped(fixtures.AcmeID), v.Row.ID)
	require.NoError(t, err)
	assert.True(t, got.Config.SES.ConfigurationSetMissing, "the warning stays until a save finds the set")

	env.SES.SetConfigurationSetMissing(false)
	require.NoError(t, editSES(env, v.Row.ID, integrations.SESConfig{
		Region: "eu-west-1", AccessKeyID: "AKIA1234", From: "news@acme.test", ConfigurationSet: "marketing",
	}))
	got, err = testhelper.Module[*integrations.Module](env).View(t.Context(), env.DB.Scoped(fixtures.AcmeID), v.Row.ID)
	require.NoError(t, err)
	assert.False(t, got.Config.SES.ConfigurationSetMissing, "a save that finds the set lifts the warning")
}

func TestACheckWithoutAnAnswerSavesWithoutAWarning(t *testing.T) {
	env := testhelper.Setup(t)
	env.SES.SetConfigurationSetErr(errors.New("AccessDeniedException: ses:GetConfigurationSet"))

	v := createSESWithSet(t, env, "marketing")

	assert.Equal(t, "marketing", v.Config.SES.ConfigurationSet)
	assert.False(t, v.Config.SES.ConfigurationSetMissing)
}

func TestAnEditWithoutTheSetClearsIt(t *testing.T) {
	env := testhelper.Setup(t)
	env.SES.SetConfigurationSetMissing(true)
	v := createSESWithSet(t, env, "marketing")

	require.NoError(t, editSES(env, v.Row.ID, integrations.SESConfig{Region: "eu-west-1", AccessKeyID: "AKIA1234", From: "news@acme.test"}))

	got := stored[ses.Config](t, env, v.Row.ID)
	assert.Empty(t, got.ConfigurationSet, "the account's default pool sends again")
	assert.False(t, got.ConfigurationSetMissing)
}

func TestAConfigurationSetSESWouldNotTakeIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := testhelper.Module[*integrations.Module](env).Create(t.Context(), env.DB.Scoped(fixtures.AcmeID), integrations.CreateInput{
		Name: "Marketing",
		Config: integrations.ConfigInput{SES: &integrations.SESConfig{
			Region: "eu-west-1", AccessKeyID: "AKIA1234", SecretAccessKey: "s", From: "news@acme.test", ConfigurationSet: "my set",
		}},
	})

	var refusal *validation.RefusalError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, "config.configurationSet", refusal.Field())
}
