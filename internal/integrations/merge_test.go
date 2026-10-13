package integrations

import (
	"encoding/json"
	"testing"

	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/messaging/ses"
	"github.com/getsphericon/sphericon/internal/messaging/smtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func marshal(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

func TestMergeSecretsSMTP(t *testing.T) {
	prev := marshal(t, smtp.Config{Host: "smtp.acme", Port: 587, Username: "u", Password: "stored", From: "a@b.c"})

	// Same destination: a blank password keeps the stored secret; other fields update.
	merged, err := mergeSecrets(integration.ProviderSMTP, marshal(t, smtp.Config{Host: "smtp.acme", Port: 587, Username: "u", From: "new@b.c"}), prev)
	require.NoError(t, err)
	var got smtp.Config
	require.NoError(t, json.Unmarshal(merged, &got))
	assert.Equal(t, "stored", got.Password)
	assert.Equal(t, "new@b.c", got.From)

	// A supplied password overrides the stored one, wherever it points.
	merged, err = mergeSecrets(integration.ProviderSMTP, marshal(t, smtp.Config{Host: "other", Port: 25, Password: "rotated"}), prev)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(merged, &got))
	assert.Equal(t, "rotated", got.Password)

	// A moved destination with a blank password is refused: the stored secret never follows it.
	for name, moved := range map[string]smtp.Config{
		"host":     {Host: "attacker", Port: 587, Username: "u"},
		"port":     {Host: "smtp.acme", Port: 25, Username: "u"},
		"username": {Host: "smtp.acme", Port: 587, Username: "other"},
	} {
		_, err := mergeSecrets(integration.ProviderSMTP, marshal(t, moved), prev)
		var verr *ValidationError
		assert.ErrorAs(t, err, &verr, name)
	}
}

func TestMergeSecretsSMTPWithoutStoredPassword(t *testing.T) {
	// An unauthenticated relay has nothing to leak, so it may move freely.
	prev := marshal(t, smtp.Config{Host: "relay", Port: 25})
	_, err := mergeSecrets(integration.ProviderSMTP, marshal(t, smtp.Config{Host: "relay2", Port: 25}), prev)
	assert.NoError(t, err)
}

func TestMergeSecretsSES(t *testing.T) {
	prev := marshal(t, ses.Config{Region: "eu-west-1", AccessKeyID: "AKIA", SecretAccessKey: "stored", Endpoint: "https://ses.acme"})

	merged, err := mergeSecrets(integration.ProviderSes, marshal(t, ses.Config{Region: "eu-west-1", AccessKeyID: "AKIA", Endpoint: "https://ses.acme", From: "new@b.c"}), prev)
	require.NoError(t, err)
	var got ses.Config
	require.NoError(t, json.Unmarshal(merged, &got))
	assert.Equal(t, "stored", got.SecretAccessKey)
	assert.Equal(t, "new@b.c", got.From)

	for name, moved := range map[string]ses.Config{
		"region":   {Region: "us-east-1", AccessKeyID: "AKIA", Endpoint: "https://ses.acme"},
		"key id":   {Region: "eu-west-1", AccessKeyID: "AKIB", Endpoint: "https://ses.acme"},
		"endpoint": {Region: "eu-west-1", AccessKeyID: "AKIA", Endpoint: "https://attacker"},
		"cleared":  {Region: "eu-west-1", AccessKeyID: "AKIA"},
	} {
		_, err := mergeSecrets(integration.ProviderSes, marshal(t, moved), prev)
		var verr *ValidationError
		assert.ErrorAs(t, err, &verr, name)
	}
}
