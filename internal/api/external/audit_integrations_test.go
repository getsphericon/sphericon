package external_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/apitoken"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// tokenActor is the actor of the one fresh token `ExternalScoped` just minted.
func tokenActor(t *testing.T, env *testhelper.TestEnv) events.Actor {
	t.Helper()
	tok, err := env.DB.ApiToken.Query().Where(apitoken.Name("actor-token"), apitoken.WorkspaceID(fixtures.AcmeID)).Only(context.Background())
	require.NoError(t, err)
	return events.Actor{Kind: events.ActorAPIToken, ID: strconv.FormatInt(tok.ID, 10), Name: tok.Name}
}

// assertTokenAudited checks the outbox holds exactly the wanted actions, in order, for one
// target: every entry is attributed to the token, and no secret (nor any sealed value the
// row held) reaches an entry; a sensitive field appears in a diff only as "changed".
func assertTokenAudited(t *testing.T, env *testhelper.TestEnv, targetID externalapi.EntityId, targetName string, wantActions, sensitiveFields, secrets []string) []*events.AuditEntry {
	t.Helper()
	got := env.OutboxEvents(t, events.NameAuditEntry)
	require.Len(t, got, len(wantActions))
	actor := tokenActor(t, env)
	entries := make([]*events.AuditEntry, 0, len(got))
	actions := make([]string, 0, len(got))
	for _, ev := range got {
		e, ok := ev.(*events.AuditEntry)
		require.Truef(t, ok, "got %T", ev)
		entries = append(entries, e)
		actions = append(actions, e.Action)
		assert.Equal(t, actor, e.Actor)
		assert.Equal(t, strconv.FormatInt(int64(targetID), 10), e.TargetID)
		assert.Equal(t, targetName, e.TargetName)
		raw, err := json.Marshal(e)
		require.NoError(t, err)
		for _, secret := range secrets {
			assert.NotContains(t, string(raw), secret)
		}
		for _, field := range sensitiveFields {
			if v, ok := e.Diff[field]; ok {
				assert.Equal(t, "changed", v, "a sensitive field is redacted")
			}
		}
	}
	assert.Equal(t, wantActions, actions)
	return entries
}

// Integration writes through /api are audited under the token, and the credentials never
// reach the entry.
func TestExternalIntegrationWritesAreAuditedUnderTheToken(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	scoped := env.DB.Scoped(fixtures.AcmeID)

	created := createSMTP(t, c, "Audited mailer", false)
	row, err := scoped.Integration().Get(ctx, int64(created.ID))
	require.NoError(t, err)

	_, err = c.IntegrationsUpdate(ctx, &externalapi.UpdateIntegrationInput{Config: externalapi.NewOptNilIntegrationConfigInput(smtpInput("smtp2.example.com"))},
		externalapi.IntegrationsUpdateParams{ID: created.ID})
	require.NoError(t, err)
	updated, err := scoped.Integration().Get(ctx, row.ID)
	require.NoError(t, err)
	require.NotEqual(t, row.ConfigEncrypted, updated.ConfigEncrypted, "the update sealed a new config")
	del, err := c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: created.ID})
	require.NoError(t, err)
	require.IsType(t, &externalapi.IntegrationsDeleteNoContent{}, del)

	assertTokenAudited(t, env, created.ID, "Audited mailer",
		[]string{"integration.create", "integration.update", "integration.delete"},
		[]string{"config_encrypted"},
		[]string{smtpSecret, row.ConfigEncrypted, updated.ConfigEncrypted})
}

// Sending domain writes through /api, are audited under the
// token, the DKIM key never reaches the entry, and the owner reads the entries on /site.
func TestExternalSendingDomainWritesAreAuditedUnderTheToken(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "sending_domains:read", "sending_domains:write")
	scoped := env.DB.Scoped(fixtures.AcmeID)

	created := createDomain(t, c, externalapi.CreateSendingDomainInput{Domain: "audited.example.com"})
	row, err := scoped.SendingDomain().Get(ctx, int64(created.ID))
	require.NoError(t, err)

	_, err = c.SendingDomainsUpdate(ctx, &externalapi.UpdateSendingDomainInput{DkimSelector: externalapi.NewOptString("s2")},
		externalapi.SendingDomainsUpdateParams{ID: created.ID})
	require.NoError(t, err)
	updated, err := scoped.SendingDomain().Get(ctx, row.ID)
	require.NoError(t, err)
	// A verify only records a check result; it is not an audited edit.
	ver, err := c.SendingDomainsVerify(ctx, externalapi.SendingDomainsVerifyParams{ID: created.ID})
	require.NoError(t, err)
	require.IsType(t, &externalapi.SendingDomainResource{}, ver)
	del, err := c.SendingDomainsDelete(ctx, externalapi.SendingDomainsDeleteParams{ID: created.ID})
	require.NoError(t, err)
	require.IsType(t, &externalapi.SendingDomainsDeleteNoContent{}, del)

	assertTokenAudited(t, env, created.ID, "audited.example.com",
		[]string{"sending_domain.create", "sending_domain.update", "sending_domain.delete"},
		[]string{"dkim_private_key_encrypted"},
		[]string{"PRIVATE KEY", row.DkimPrivateKeyEncrypted, updated.DkimPrivateKeyEncrypted})

	env.DeliverToEE(t)
	list, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteAuditList(ctx, siteapi.SiteAuditListParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	page, ok := list.(*siteapi.SiteAuditEntryList)
	require.Truef(t, ok, "got %T", list)
	domainEntries := lo.Filter(page.Items, func(item siteapi.SiteAuditEntryResource, _ int) bool {
		return strings.HasPrefix(item.Action, "sending_domain.")
	})
	for _, item := range domainEntries {
		assert.Equal(t, siteapi.SiteAuditActorKindAPIToken, item.Actor.Kind)
	}
	assert.Len(t, domainEntries, 3)
}
