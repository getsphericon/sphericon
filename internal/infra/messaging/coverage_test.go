package messaging_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/registry"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestDomainOf(t *testing.T) {
	assert.Equal(t, "mail.acme.com", messaging.DomainOf("Hi@Mail.ACME.com"))
	assert.Empty(t, messaging.DomainOf("no-at-sign"))
	assert.Empty(t, messaging.DomainOf("trailing@"))
}

func TestCatalogValidate(t *testing.T) {
	boom := errors.New("boom")
	cat := messaging.NewCatalog(
		messaging.ProviderDescriptor{
			Provider: providers.SMTP,
			Validate: func([]byte) error { return boom },
		},
		messaging.ProviderDescriptor{Provider: providers.SES},
	)

	require.ErrorIs(t, cat.Validate(providers.SMTP, nil), boom)
	require.NoError(t, cat.Validate(providers.SES, nil), "no Validate hook means nothing to check")
	assert.ErrorContains(t, cat.Validate(providers.Twilio, nil), "unknown provider")
}

func TestCatalogBuildEmail(t *testing.T) {
	boom := errors.New("build failed")
	cat := messaging.NewCatalog(
		messaging.ProviderDescriptor{
			Provider: providers.SMTP,
			Build: func([]byte, messaging.Deps) (any, error) {
				return &EmailSenderMock{SendFunc: func(context.Context, messaging.EmailMessage) (messaging.Receipt, error) {
					return messaging.Receipt{MessageID: "stub"}, nil
				}}, nil
			},
		},
		messaging.ProviderDescriptor{
			Provider: providers.SES,
			Build:    func([]byte, messaging.Deps) (any, error) { return nil, boom },
		},
		messaging.ProviderDescriptor{
			Provider: providers.Inbox,
			Build:    func([]byte, messaging.Deps) (any, error) { return "not a sender", nil },
		},
	)

	sender, err := cat.BuildEmail(providers.SMTP, nil, messaging.Deps{})
	require.NoError(t, err)
	assert.NotNil(t, sender, "built sender is wrapped by the outcome counter, so assert behaviour in metrics_test.go")

	_, err = cat.BuildEmail(providers.SES, nil, messaging.Deps{})
	require.ErrorIs(t, err, boom)

	_, err = cat.BuildEmail(providers.Inbox, nil, messaging.Deps{})
	require.ErrorContains(t, err, "did not build an EmailSender")

	_, err = cat.BuildEmail(providers.Provider("missing"), nil, messaging.Deps{})
	assert.ErrorContains(t, err, "unknown provider")
}

func TestBuildMIMEInvalidAddresses(t *testing.T) {
	_, err := messaging.BuildMIME(messaging.EmailMessage{From: "not-an-address", FromName: "Acme", To: "a@b.com", Text: "x"})
	require.ErrorContains(t, err, "invalid from address")

	_, err = messaging.BuildMIME(messaging.EmailMessage{From: "a@b.com", To: "not-an-address", Text: "x"})
	assert.ErrorContains(t, err, "invalid to address")
}

type failingKeys struct{ err error }

func (f failingKeys) Key(context.Context, string) (*messaging.DKIMKey, error) {
	return nil, f.err
}

func TestBuildSignedMessagePropagatesBuildAndKeyErrors(t *testing.T) {
	_, err := messaging.BuildSignedMessage(context.Background(),
		messaging.EmailMessage{From: "bad", To: "a@b.com", Text: "x"}, failingKeys{})
	require.ErrorContains(t, err, "invalid from address")

	boom := errors.New("keys down")
	_, err = messaging.BuildSignedMessage(context.Background(),
		messaging.EmailMessage{From: "a@b.com", To: "c@d.com", Text: "x"}, failingKeys{err: boom})
	assert.ErrorIs(t, err, boom)
}

func TestDKIMKeysEmptyDomain(t *testing.T) {
	env := testhelper.Setup(t)
	dk, err := messaging.NewDKIMKeys(env.DB.Scoped(fixtures.AcmeID), env.Cipher).Key(context.Background(), "no-at-sign")
	require.NoError(t, err)
	assert.Nil(t, dk)
}

func TestDKIMKeysQueryError(t *testing.T) {
	env := testhelper.Setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := messaging.NewDKIMKeys(env.DB.Scoped(fixtures.AcmeID), env.Cipher).Key(ctx, "hi@mail.acme.com")
	assert.Error(t, err)
}

func TestDKIMKeysBrokenKeyMaterial(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	cipher := env.Cipher

	// Ciphertext that does not decrypt under the workspace cipher.
	env.DB.SendingDomain.UpdateOneID(fixtures.SendingDomainVerifiedID).SetDkimPrivateKeyEncrypted("not-a-ciphertext").ExecX(ctx)
	_, err := messaging.NewDKIMKeys(env.DB.Scoped(fixtures.AcmeID), cipher).Key(ctx, "hi@"+fixtures.SendingDomainVerifiedDomain)
	require.ErrorContains(t, err, "decrypt dkim key")

	// Decrypts fine but is not a PEM private key.
	sealed, err := cipher.Encrypt([]byte("definitely not PEM"))
	require.NoError(t, err)
	env.DB.SendingDomain.UpdateOneID(fixtures.SendingDomainVerifiedID).SetDkimPrivateKeyEncrypted(sealed).ExecX(ctx)
	_, err = messaging.NewDKIMKeys(env.DB.Scoped(fixtures.AcmeID), cipher).Key(ctx, "hi@"+fixtures.SendingDomainVerifiedDomain)
	assert.ErrorContains(t, err, "parse dkim key")
}

func TestHasVerifiedSendingDomain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	ok, err := messaging.HasVerifiedSendingDomain(ctx, env.DB.Scoped(fixtures.AcmeID), "hi@mail.acme.com")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = messaging.HasVerifiedSendingDomain(ctx, env.DB.Scoped(fixtures.AcmeID), "hi@news.acme.com")
	require.NoError(t, err)
	assert.False(t, ok, "unverified domain")

	ok, err = messaging.HasVerifiedSendingDomain(ctx, env.DB.Scoped(fixtures.GlobexID), "hi@mail.acme.com")
	require.NoError(t, err)
	assert.False(t, ok, "another workspace's domain")

	ok, err = messaging.HasVerifiedSendingDomain(ctx, env.DB.Scoped(fixtures.AcmeID), "malformed")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestResolverErrors(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	resolver := messaging.NewResolver(env.Cipher, registry.Default())

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err := messaging.Integration(canceled, env.DB.Scoped(fixtures.AcmeID), channels.Email, nil)
	require.Error(t, err)
	require.NotErrorIs(t, err, messaging.ErrNoProvider)

	env.DB.Integration.UpdateOneID(fixtures.IntegrationGlobexID).SetConfigEncrypted("not-a-ciphertext").SetIsDefault(true).ExecX(ctx)
	integ, err := messaging.Integration(ctx, env.DB.Scoped(fixtures.GlobexID), channels.Email, nil)
	require.NoError(t, err)
	_, err = resolver.EmailSender(ctx, env.DB.Scoped(fixtures.GlobexID), integ)
	assert.ErrorContains(t, err, "decrypt integration")
}
