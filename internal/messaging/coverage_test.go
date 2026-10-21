package messaging_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wneessen/go-mail"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/registry"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestFirstNonEmpty(t *testing.T) {
	assert.Equal(t, "b", messaging.FirstNonEmpty("", "b", "c"))
	assert.Empty(t, messaging.FirstNonEmpty("", ""))
	assert.Empty(t, messaging.FirstNonEmpty())
}

func TestFormatSender(t *testing.T) {
	assert.Equal(t, "Acme <hi@acme.com>", messaging.FormatSender("hi@acme.com", "Acme"))
	assert.Equal(t, "hi@acme.com", messaging.FormatSender("hi@acme.com", ""))
}

func TestDomainOf(t *testing.T) {
	assert.Equal(t, "mail.acme.com", messaging.DomainOf("Hi@Mail.ACME.com"))
	assert.Empty(t, messaging.DomainOf("no-at-sign"))
	assert.Empty(t, messaging.DomainOf("trailing@"))
}

func TestCatalogChannelOf(t *testing.T) {
	cat := registry.Default()

	ch, ok := cat.ChannelOf(messaging.ProviderSES)
	assert.True(t, ok)
	assert.Equal(t, messaging.ChannelEmail, ch)

	_, ok = cat.ChannelOf(messaging.Provider("carrier-pigeon"))
	assert.False(t, ok)
}

func TestCatalogValidate(t *testing.T) {
	boom := errors.New("boom")
	cat := messaging.NewCatalog(
		messaging.ProviderDescriptor{
			Channel: messaging.ChannelEmail, Provider: messaging.ProviderSMTP,
			Validate: func([]byte) error { return boom },
		},
		messaging.ProviderDescriptor{Channel: messaging.ChannelEmail, Provider: messaging.ProviderSES},
	)

	require.ErrorIs(t, cat.Validate(messaging.ChannelEmail, messaging.ProviderSMTP, nil), boom)
	require.NoError(t, cat.Validate(messaging.ChannelEmail, messaging.ProviderSES, nil), "no Validate hook means nothing to check")
	assert.ErrorContains(t, cat.Validate(messaging.ChannelSMS, messaging.ProviderSES, nil), "unknown provider")
}

func TestCatalogBuildEmail(t *testing.T) {
	boom := errors.New("build failed")
	cat := messaging.NewCatalog(
		messaging.ProviderDescriptor{
			Channel: messaging.ChannelEmail, Provider: messaging.ProviderSMTP,
			Build: func([]byte, messaging.Signer) (any, error) {
				return &EmailSenderMock{SendFunc: func(context.Context, messaging.EmailMessage) (messaging.Receipt, error) {
					return messaging.Receipt{MessageID: "stub"}, nil
				}}, nil
			},
		},
		messaging.ProviderDescriptor{
			Channel: messaging.ChannelEmail, Provider: messaging.ProviderSES,
			Build: func([]byte, messaging.Signer) (any, error) { return nil, boom },
		},
		messaging.ProviderDescriptor{
			Channel: messaging.ChannelEmail, Provider: messaging.Provider("wrong"),
			Build: func([]byte, messaging.Signer) (any, error) { return "not a sender", nil },
		},
	)

	sender, err := cat.BuildEmail(messaging.ProviderSMTP, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, sender, "built sender is wrapped by the outcome counter, so assert behaviour in metrics_test.go")

	_, err = cat.BuildEmail(messaging.ProviderSES, nil, nil)
	require.ErrorIs(t, err, boom)

	_, err = cat.BuildEmail(messaging.Provider("wrong"), nil, nil)
	require.ErrorContains(t, err, "did not build an EmailSender")

	_, err = cat.BuildEmail(messaging.Provider("missing"), nil, nil)
	assert.ErrorContains(t, err, "unknown email provider")
}

func TestBuildMIMEInvalidAddresses(t *testing.T) {
	_, err := messaging.BuildMIME(messaging.EmailMessage{From: "not-an-address", FromName: "Acme", To: "a@b.com", Text: "x"})
	require.ErrorContains(t, err, "invalid from address")

	_, err = messaging.BuildMIME(messaging.EmailMessage{From: "a@b.com", To: "not-an-address", Text: "x"})
	assert.ErrorContains(t, err, "invalid to address")
}

type failingSigner struct{ err error }

func (f failingSigner) DKIMSigner(context.Context, string) (*mail.DKIMSigner, error) {
	return nil, f.err
}

func TestBuildSignedMIMEPropagatesBuildAndSignerErrors(t *testing.T) {
	_, err := messaging.BuildSignedMIME(context.Background(),
		messaging.EmailMessage{From: "bad", To: "a@b.com", Text: "x"}, failingSigner{})
	require.ErrorContains(t, err, "invalid from address")

	boom := errors.New("signer down")
	_, err = messaging.BuildSignedMIME(context.Background(),
		messaging.EmailMessage{From: "a@b.com", To: "c@d.com", Text: "x"}, failingSigner{err: boom})
	assert.ErrorIs(t, err, boom)
}

func TestDKIMSignerEmptyDomain(t *testing.T) {
	env := testhelper.Setup(t)
	dk, err := messaging.NewDKIMSigner(env.DB.Scoped(fixtures.AcmeID), envCipher(t)).DKIMSigner(context.Background(), "no-at-sign")
	require.NoError(t, err)
	assert.Nil(t, dk)
}

func TestDKIMSignerQueryError(t *testing.T) {
	env := testhelper.Setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := messaging.NewDKIMSigner(env.DB.Scoped(fixtures.AcmeID), envCipher(t)).DKIMSigner(ctx, "hi@mail.acme.com")
	assert.Error(t, err)
}

func TestDKIMSignerBrokenKeyMaterial(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	cipher := envCipher(t)

	// Ciphertext that does not decrypt under the workspace cipher.
	_, err := env.DB.SendingDomain.Create().
		SetWorkspaceID(1).SetDomain("garbled.acme.com").SetDkimSelector("sphericon").
		SetDkimPrivateKeyEncrypted("not-a-ciphertext").SetDkimPublicKey("v=DKIM1").SetVerified(true).SetVerifiedAt(time.Now()).
		Save(ctx)
	require.NoError(t, err)
	_, err = messaging.NewDKIMSigner(env.DB.Scoped(fixtures.AcmeID), cipher).DKIMSigner(ctx, "hi@garbled.acme.com")
	require.ErrorContains(t, err, "decrypt dkim key")

	// Decrypts fine but is not a PEM private key.
	sealed, err := cipher.Encrypt([]byte("definitely not PEM"))
	require.NoError(t, err)
	_, err = env.DB.SendingDomain.Create().
		SetWorkspaceID(1).SetDomain("nopem.acme.com").SetDkimSelector("sphericon").
		SetDkimPrivateKeyEncrypted(sealed).SetDkimPublicKey("v=DKIM1").SetVerified(true).SetVerifiedAt(time.Now()).
		Save(ctx)
	require.NoError(t, err)
	_, err = messaging.NewDKIMSigner(env.DB.Scoped(fixtures.AcmeID), cipher).DKIMSigner(ctx, "hi@nopem.acme.com")
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
	resolver := messaging.NewResolver(envCipher(t), registry.Default())

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err := resolver.EmailSender(canceled, env.DB.Scoped(fixtures.AcmeID))
	require.Error(t, err)
	require.NotErrorIs(t, err, messaging.ErrNoProvider)

	_, err = env.DB.Integration.Create().
		SetWorkspaceID(fixtures.GlobexID).SetName("broken").SetProvider("smtp").
		SetConfigEncrypted("not-a-ciphertext").SetIngestKey("omik_broken").SetIsDefault(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = resolver.EmailSender(ctx, env.DB.Scoped(fixtures.GlobexID))
	assert.ErrorContains(t, err, "decrypt integration")
}
