package webhooks_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/webhooks"
)

type license bool

func (l license) Licensed() bool { return bool(l) }

func module(env *testhelper.TestEnv, lic webhooks.Licensing) *webhooks.Module {
	return webhooks.New(env.Cipher, lic)
}

// Fixtures (workspace 1): endpoint 100 (codebasics, filtered, secret whsec_Mf...)
// and 101 (disabled). Workspace 2 owns endpoint 900.
func TestCreateReturnsTheEndpointWithItsDecryptedStandardWebhooksSecret(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()

	got, err := module(env, license(true)).Create(ctx, acme, webhooks.CreateInput{
		URL: "https://example.com/hook", EventTypes: []string{"contact.created"},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/hook", got.URL)
	assert.True(t, got.Enabled)
	require.True(t, strings.HasPrefix(got.Secret, "whsec_"))
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(got.Secret, "whsec_"))
	require.NoError(t, err)
	assert.Len(t, raw, 24)

	stored, err := acme.WebhookEndpoint().Get(ctx, got.ID)
	require.NoError(t, err)
	assert.NotContains(t, string(stored.SecretEncrypted), got.Secret, "stored sealed")

	again, err := module(env, license(true)).Get(ctx, acme, got.ID)
	require.NoError(t, err)
	assert.Equal(t, got.Secret, again.Secret)
}

func TestCreateHonoursEnabledFalse(t *testing.T) {
	env := testhelper.Setup(t)
	off := false
	got, err := module(env, nil).Create(context.Background(), env.DB.Scoped(fixtures.AcmeID),
		webhooks.CreateInput{URL: "https://example.com/off", Enabled: &off})
	require.NoError(t, err)
	assert.False(t, got.Enabled)
}

func TestInvalidURLIsRefusedOnCreateAndUpdate(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := module(env, nil)
	ctx := context.Background()

	for _, url := range []string{"ftp://example.com/hook", "example.com/hook", "https://", "/relative", "", "http://%zz", "not-a-url"} {
		_, err := m.Create(ctx, acme, webhooks.CreateInput{URL: url})
		assert.ErrorIs(t, err, webhooks.ErrInvalidURL, url)

		bad := url
		_, err = m.Update(ctx, acme, fixtures.WebhookCodebasicsID, webhooks.UpdateInput{URL: &bad})
		assert.ErrorIs(t, err, webhooks.ErrInvalidURL, url)
	}
	ok := "http://example.com:8080/x"
	got, err := m.Update(ctx, acme, fixtures.WebhookCodebasicsID, webhooks.UpdateInput{URL: &ok})
	require.NoError(t, err)
	assert.Equal(t, ok, got.URL)
}

func TestUpdateChangesOnlyTheGivenFieldsAndKeepsTheSecret(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := module(env, nil)
	ctx := context.Background()

	before, err := m.Get(ctx, acme, fixtures.WebhookCodebasicsID)
	require.NoError(t, err)
	assert.Equal(t, "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", before.Secret)

	off := false
	got, err := m.Update(ctx, acme, fixtures.WebhookCodebasicsID, webhooks.UpdateInput{Enabled: &off})
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.Equal(t, before.URL, got.URL)
	assert.Equal(t, before.EventTypes, got.EventTypes, "nil event types leave the filter alone")
	assert.Equal(t, before.Secret, got.Secret)

	got, err = m.Update(ctx, acme, fixtures.WebhookCodebasicsID, webhooks.UpdateInput{EventTypes: []string{"email.opened"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"email.opened"}, got.EventTypes)
}

func TestAuditEntryNeedsALicenseOnCreateAndUpdate(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()
	audit := []string{events.NameAuditEntry}

	for name, lic := range map[string]webhooks.Licensing{"nil": nil, "unlicensed": license(false)} {
		m := module(env, lic)
		_, err := m.Create(ctx, acme, webhooks.CreateInput{URL: "https://example.com/a", EventTypes: audit})
		assert.ErrorIs(t, err, webhooks.ErrAuditNeedsLicense, name)
		_, err = m.Update(ctx, acme, fixtures.WebhookCodebasicsID, webhooks.UpdateInput{EventTypes: audit})
		assert.ErrorIs(t, err, webhooks.ErrAuditNeedsLicense, name)
	}

	m := module(env, license(true))
	_, err := m.Create(ctx, acme, webhooks.CreateInput{URL: "https://example.com/a", EventTypes: audit})
	require.NoError(t, err)
	_, err = m.Update(ctx, acme, fixtures.WebhookCodebasicsID, webhooks.UpdateInput{EventTypes: audit})
	require.NoError(t, err)
}

func TestListPagesTheWorkspaceAscendingByID(t *testing.T) {
	env := testhelper.Setup(t)
	m := module(env, nil)
	ctx := context.Background()

	page, err := m.List(ctx, env.DB.Scoped(fixtures.AcmeID), pagination.Params{Page: 1, PageSize: 1})
	require.NoError(t, err)
	total, err := env.DB.Scoped(fixtures.AcmeID).WebhookEndpoint().Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, total, page.TotalItems)
	assert.Equal(t, total, page.TotalPages)
	require.Len(t, page.Items, 1)
	assert.Equal(t, int64(fixtures.WebhookCodebasicsID), page.Items[0].ID)
	assert.Equal(t, "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", page.Items[0].Secret)

	second, err := m.List(ctx, env.DB.Scoped(fixtures.AcmeID), pagination.Params{Page: 2, PageSize: 1})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Equal(t, int64(fixtures.WebhookCodebasicsDisabledID), second.Items[0].ID)
}

func TestDeleteAndWorkspaceIsolation(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := module(env, nil)
	ctx := context.Background()

	_, err := m.Get(ctx, acme, fixtures.WebhookGlobexID)
	assert.True(t, ent.IsNotFound(err))
	_, err = m.Update(ctx, acme, fixtures.WebhookGlobexID, webhooks.UpdateInput{})
	assert.True(t, ent.IsNotFound(err))
	assert.True(t, ent.IsNotFound(m.Delete(ctx, acme, fixtures.WebhookGlobexID)))

	require.NoError(t, m.Delete(ctx, acme, fixtures.WebhookCodebasicsDisabledID))
	_, err = m.Get(ctx, acme, fixtures.WebhookCodebasicsDisabledID)
	assert.True(t, ent.IsNotFound(err))
}

// Enabling an endpoint that turned itself off starts it fresh: the reason and the failure
// state are gone. Disabling it by hand drops the reason too, since the owner decided now.
func TestEnablingClearsTheFailureStateAndTheDisabledReason(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := module(env, nil)
	ctx := context.Background()
	on := true

	before, err := acme.WebhookEndpoint().Get(ctx, fixtures.WebhookCodebasicsGoneID)
	require.NoError(t, err)
	require.NotNil(t, before.DisabledReason)
	require.NotNil(t, before.ConsecutiveFailures)

	got, err := m.Update(ctx, acme, fixtures.WebhookCodebasicsGoneID, webhooks.UpdateInput{Enabled: &on})
	require.NoError(t, err)
	assert.True(t, got.Enabled)
	assert.Nil(t, got.DisabledReason)
	assert.Nil(t, got.ConsecutiveFailures)
	assert.Nil(t, got.FailingSince)
	assert.Nil(t, got.LastErrorKind)
	assert.Nil(t, got.LastErrorStatus)
	assert.Nil(t, got.LastFailureAt)
}
