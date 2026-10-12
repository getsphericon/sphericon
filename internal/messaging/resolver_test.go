package messaging_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/registry"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCipher(t *testing.T) *secrets.Cipher {
	t.Helper()
	ks, err := secrets.GenerateKeysetBase64()
	require.NoError(t, err)
	c, err := secrets.NewCipher(ks)
	require.NoError(t, err)
	return c
}

// envCipher builds the cipher over the test environment's ENCRYPTION_KEY — the
// same key the fixtures were sealed with — so fixture-stored configs decrypt.
func envCipher(t *testing.T) *secrets.Cipher {
	t.Helper()
	cfg, err := config.Load("test")
	require.NoError(t, err)
	c, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	return c
}

// Resolver loads the workspace's default email integration, decrypts it and
// builds a live EmailSender via the catalog. Workspace 1 ("acme") is seeded with
// a default SMTP integration.
func TestResolverEmailSender(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	resolver := messaging.NewResolver(envCipher(t), registry.Default())

	sender, err := resolver.EmailSender(ctx, env.DB.Scoped(fixtures.AcmeID))
	require.NoError(t, err)
	assert.NotNil(t, sender)
}

// A workspace without any integration has no default to resolve.
func TestResolverNoDefault(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// The Globex fixture tenant has no integration.
	resolver := messaging.NewResolver(newTestCipher(t), registry.Default())

	_, err := resolver.EmailSender(ctx, env.DB.Scoped(fixtures.GlobexID))
	assert.ErrorIs(t, err, messaging.ErrNoProvider)
}
