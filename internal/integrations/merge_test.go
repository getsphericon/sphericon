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

func TestMergeSecretsSMTP(t *testing.T) {
	prev, err := json.Marshal(smtp.Config{Host: "old", Port: 587, Password: "stored"})
	require.NoError(t, err)

	// Blank password on update keeps the stored secret; other fields update.
	next, err := json.Marshal(smtp.Config{Host: "new", Port: 25, Password: ""})
	require.NoError(t, err)
	merged, err := mergeSecrets(integration.ProviderSMTP, next, prev)
	require.NoError(t, err)
	var got smtp.Config
	require.NoError(t, json.Unmarshal(merged, &got))
	assert.Equal(t, "stored", got.Password, "blank password keeps the stored secret")
	assert.Equal(t, "new", got.Host)
	assert.Equal(t, 25, got.Port)

	// A supplied password overrides the stored one.
	next2, err := json.Marshal(smtp.Config{Host: "new", Port: 25, Password: "rotated"})
	require.NoError(t, err)
	merged2, err := mergeSecrets(integration.ProviderSMTP, next2, prev)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(merged2, &got))
	assert.Equal(t, "rotated", got.Password)
}

func TestMergeSecretsSES(t *testing.T) {
	prev, err := json.Marshal(ses.Config{Region: "eu-west-1", AccessKeyID: "AKIA", SecretAccessKey: "stored"})
	require.NoError(t, err)

	next, err := json.Marshal(ses.Config{Region: "us-east-1", AccessKeyID: "AKIA", SecretAccessKey: ""})
	require.NoError(t, err)
	merged, err := mergeSecrets(integration.ProviderSes, next, prev)
	require.NoError(t, err)
	var got ses.Config
	require.NoError(t, json.Unmarshal(merged, &got))
	assert.Equal(t, "stored", got.SecretAccessKey, "blank secret keeps the stored value")
	assert.Equal(t, "us-east-1", got.Region)
}
