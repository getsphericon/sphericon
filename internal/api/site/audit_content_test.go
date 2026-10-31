package site_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/go-faster/jx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Content and audience entities emit entries with a diff, a name snapshot and the User.
func TestSiteContentChangesAreAudited(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	created, err := c.SiteTemplatesCreate(ctx,
		&siteapi.CreateTemplateInput{Name: "Audited template", Subject: siteapi.NewOptString("Hi")},
		siteapi.SiteTemplatesCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	tmplID := created.(*siteapi.SiteEmailTemplateResource).ID

	_, err = c.SiteTemplatesUpdate(ctx, &siteapi.UpdateTemplateInput{Subject: siteapi.NewOptString("Hello")},
		siteapi.SiteTemplatesUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: tmplID})
	require.NoError(t, err)
	_, err = c.SiteTemplatesDelete(ctx, siteapi.SiteTemplatesDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: tmplID})
	require.NoError(t, err)

	_, err = c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{Name: "Audited segment", Condition: testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[]}`)},
		siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	byAction := map[string]*events.AuditEntry{}
	for _, ev := range env.OutboxEvents(t, events.NameAuditEntry) {
		e := ev.(*events.AuditEntry)
		byAction[e.Action] = e
	}
	require.Contains(t, byAction, "template.create")
	assert.Equal(t, "Audited template", byAction["template.create"].TargetName)
	assert.Equal(t, events.ActorUser, byAction["template.create"].Actor.Kind)
	require.Contains(t, byAction, "template.update")
	assert.Equal(t, map[string]any{"subject": map[string]any{"from": "Hi", "to": "Hello"}}, byAction["template.update"].Diff)
	require.Contains(t, byAction, "template.delete")
	require.Contains(t, byAction, "segment.create")
	assert.Equal(t, "Audited segment", byAction["segment.create"].TargetName)
}

// A Contact entry holds the id and the names of changed fields, never a value.
func TestSiteContactChangesCarryNamesOnly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	const email, first, secret = "leak.check@example.org", "Leakyname", "custom-secret-value"
	created, err := c.SiteContactsCreate(ctx, &siteapi.SiteCreateContactInput{
		Email:     siteapi.NewOptNilEmailAddress(email),
		FirstName: siteapi.NewOptNilString(first),
	}, siteapi.SiteContactsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	id := created.(*siteapi.SiteContactResource).ID

	_, err = c.SiteContactsUpdate(ctx, &siteapi.SiteUpdateContactInput{
		LastName:     siteapi.NewOptNilString("Lastleak"),
		CustomFields: siteapi.NewOptNilJsonObject(siteapi.JsonObject{"plan": jx.Raw(`"` + secret + `"`)}),
	}, siteapi.SiteContactsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	_, err = c.SiteContactsDelete(ctx, siteapi.SiteContactsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)

	contactEntries := lo.FilterMap(env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) (*events.AuditEntry, bool) {
		e := ev.(*events.AuditEntry)
		return e, e.TargetType == "contact"
	})
	require.Len(t, contactEntries, 3)
	assert.Equal(t, "contact.create", contactEntries[0].Action)
	assert.Equal(t, "contact.update", contactEntries[1].Action)
	assert.Equal(t, "contact.delete", contactEntries[2].Action)
	assert.Equal(t, "changed", contactEntries[0].Diff["email"])
	assert.Equal(t, "changed", contactEntries[0].Diff["first_name"])
	assert.Equal(t, map[string]any{"last_name": "changed", "custom_fields": "changed"}, contactEntries[1].Diff)

	for _, e := range contactEntries {
		assert.Equal(t, strconv.FormatInt(int64(id), 10), e.TargetID)
		assert.Empty(t, e.TargetName)
		raw, err := json.Marshal(e)
		require.NoError(t, err)
		for _, leak := range []string{email, first, "Lastleak", secret, "example.org"} {
			assert.NotContains(t, string(raw), leak)
		}
		for _, v := range e.Diff {
			assert.Equal(t, "changed", v, "a Contact diff names fields and carries no value")
		}
	}
}
