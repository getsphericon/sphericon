package integrations_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/messaging/ses"
	"github.com/getsphericon/sphericon/internal/messaging/smtp"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/validation"
)

// Secrets are redacted on read, so an edit re-sends a blank secret: blank keeps the stored one,
// but only while the destination it was saved for is unchanged.

// stored opens the config an Integration keeps, as the provider reads it.
func stored[T any](t *testing.T, env *testhelper.TestEnv, id int64) T {
	t.Helper()
	row := env.DB.Scoped(fixtures.AcmeID).Integration().Query().Where(integration.ID(id)).OnlyX(context.Background())
	plain, err := env.Cipher.Decrypt(row.ConfigEncrypted)
	require.NoError(t, err)
	var c T
	require.NoError(t, json.Unmarshal(plain, &c))
	return c
}

func editSMTP(env *testhelper.TestEnv, id int64, c integrations.SMTPConfig) error {
	_, err := env.Integrations.Update(context.Background(), env.DB.Scoped(fixtures.AcmeID), id,
		integrations.UpdateInput{Config: &integrations.ConfigInput{SMTP: &c}})
	return err
}

func editSES(env *testhelper.TestEnv, id int64, c integrations.SESConfig) error {
	_, err := env.Integrations.Update(context.Background(), env.DB.Scoped(fixtures.AcmeID), id,
		integrations.UpdateInput{Config: &integrations.ConfigInput{SES: &c}})
	return err
}

func TestASuppliedSMTPPasswordReplacesTheStoredOne(t *testing.T) {
	env := testhelper.Setup(t)
	id := int64(fixtures.IntegrationAcmeDefaultID)
	require.NoError(t, editSMTP(env, id, integrations.SMTPConfig{Host: "smtp.acme.test", Port: 587, Username: "u", Password: "stored", From: "a@acme.test"}))

	require.NoError(t, editSMTP(env, id, integrations.SMTPConfig{Host: "smtp2.acme.test", Port: 25, Password: "rotated", From: "a@acme.test"}))

	assert.Equal(t, "rotated", stored[smtp.Config](t, env, id).Password, "a supplied password goes wherever the Integration points")
}

func TestABlankSMTPPasswordKeepsTheStoredOneForTheSameDestination(t *testing.T) {
	env := testhelper.Setup(t)
	id := int64(fixtures.IntegrationAcmeDefaultID)
	require.NoError(t, editSMTP(env, id, integrations.SMTPConfig{Host: "smtp.acme.test", Port: 587, Username: "u", Password: "stored", From: "a@acme.test"}))

	require.NoError(t, editSMTP(env, id, integrations.SMTPConfig{Host: "smtp.acme.test", Port: 587, Username: "u", From: "new@acme.test"}))

	got := stored[smtp.Config](t, env, id)
	assert.Equal(t, "stored", got.Password)
	assert.Equal(t, "new@acme.test", got.From, "the other fields take the edit")
}

func TestABlankSMTPPasswordIsRefusedForAMovedDestination(t *testing.T) {
	env := testhelper.Setup(t)
	id := int64(fixtures.IntegrationAcmeDefaultID)
	require.NoError(t, editSMTP(env, id, integrations.SMTPConfig{Host: "smtp.acme.test", Port: 587, Username: "u", Password: "stored", From: "a@acme.test"}))

	for name, moved := range map[string]integrations.SMTPConfig{
		"host":     {Host: "attacker.example", Port: 587, Username: "u", From: "a@acme.test"},
		"port":     {Host: "smtp.acme.test", Port: 25, Username: "u", From: "a@acme.test"},
		"username": {Host: "smtp.acme.test", Port: 587, Username: "other", From: "a@acme.test"},
	} {
		var verr *validation.Error
		require.ErrorAs(t, editSMTP(env, id, moved), &verr, name)
	}
	assert.Equal(t, "smtp.acme.test", stored[smtp.Config](t, env, id).Host, "the stored secret never follows a moved destination")
}

// An unauthenticated relay has nothing to leak, so it may move freely.
func TestAnSMTPRelayWithoutAPasswordMovesFreely(t *testing.T) {
	env := testhelper.Setup(t)
	id := int64(fixtures.IntegrationAcmeDefaultID)
	require.NoError(t, editSMTP(env, id, integrations.SMTPConfig{Host: "relay.acme.test", Port: 25, From: "a@acme.test"}))

	require.NoError(t, editSMTP(env, id, integrations.SMTPConfig{Host: "relay2.acme.test", Port: 25, From: "a@acme.test"}))

	assert.Equal(t, "relay2.acme.test", stored[smtp.Config](t, env, id).Host)
}

const postbox = "https://postbox.cloud.yandex.net"

func TestABlankSESSecretKeepsTheStoredOneForTheSameDestination(t *testing.T) {
	env := testhelper.Setup(t)
	id := int64(fixtures.IntegrationAcmeSesID)
	require.NoError(t, editSES(env, id, integrations.SESConfig{Region: "eu-west-1", AccessKeyID: "AKIA1234", SecretAccessKey: "stored", Endpoint: postbox, From: "a@acme.test"}))

	require.NoError(t, editSES(env, id, integrations.SESConfig{Region: "eu-west-1", AccessKeyID: "AKIA1234", Endpoint: postbox, From: "new@acme.test"}))

	got := stored[ses.Config](t, env, id)
	assert.Equal(t, "stored", got.SecretAccessKey)
	assert.Equal(t, "new@acme.test", got.From, "the other fields take the edit")
}

func TestABlankSESSecretIsRefusedForAMovedDestination(t *testing.T) {
	env := testhelper.Setup(t)
	id := int64(fixtures.IntegrationAcmeSesID)
	require.NoError(t, editSES(env, id, integrations.SESConfig{Region: "eu-west-1", AccessKeyID: "AKIA1234", SecretAccessKey: "stored", Endpoint: postbox, From: "a@acme.test"}))

	for name, moved := range map[string]integrations.SESConfig{
		"region":   {Region: "us-east-1", AccessKeyID: "AKIA1234", Endpoint: postbox, From: "a@acme.test"},
		"key id":   {Region: "eu-west-1", AccessKeyID: "AKIA9999", Endpoint: postbox, From: "a@acme.test"},
		"endpoint": {Region: "eu-west-1", AccessKeyID: "AKIA1234", Endpoint: "https://attacker.example", From: "a@acme.test"},
		"cleared":  {Region: "eu-west-1", AccessKeyID: "AKIA1234", From: "a@acme.test"},
	} {
		var verr *validation.Error
		require.ErrorAs(t, editSES(env, id, moved), &verr, name)
	}
	assert.Equal(t, postbox, stored[ses.Config](t, env, id).Endpoint, "the stored secret never follows a moved destination")
}
