package site_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/mokevnin/sphericon/gen/site"
	"github.com/mokevnin/sphericon/internal/events"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/testhelper"
)

// Content and audience entities emit entries with a diff, a name snapshot and the User.
func TestSiteContentChangesAreAudited(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	created, err := c.SiteTemplatesCreate(ctx,
		&siteapi.SiteCreateEmailTemplateInput{Name: "Audited template", Subject: siteapi.NewOptString("Hi")},
		siteapi.SiteTemplatesCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	tmplID := created.(*siteapi.SiteEmailTemplateResource).ID

	_, err = c.SiteTemplatesUpdate(ctx, &siteapi.SiteUpdateEmailTemplateInput{Subject: siteapi.NewOptString("Hello")},
		siteapi.SiteTemplatesUpdateParams{Slug: fixtures.AcmeSlug, ID: tmplID})
	require.NoError(t, err)
	_, err = c.SiteTemplatesDelete(ctx, siteapi.SiteTemplatesDeleteParams{Slug: fixtures.AcmeSlug, ID: tmplID})
	require.NoError(t, err)

	_, err = c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{Name: "Audited segment", Definition: testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[]}`)},
		siteapi.SiteSegmentsCreateParams{Slug: fixtures.AcmeSlug})
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
	}, siteapi.SiteContactsCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	id := created.(*siteapi.SiteContactResource).ID

	_, err = c.SiteContactsUpdate(ctx, &siteapi.SiteUpdateContactInput{
		LastName:     siteapi.NewOptNilString("Lastleak"),
		CustomFields: siteapi.NewOptNilSiteUpdateContactInputCustomFields(siteapi.SiteUpdateContactInputCustomFields{"plan": jx.Raw(`"` + secret + `"`)}),
	}, siteapi.SiteContactsUpdateParams{Slug: fixtures.AcmeSlug, ID: id})
	require.NoError(t, err)
	_, err = c.SiteContactsDelete(ctx, siteapi.SiteContactsDeleteParams{Slug: fixtures.AcmeSlug, ID: id})
	require.NoError(t, err)

	var contactEntries []*events.AuditEntry
	for _, ev := range env.OutboxEvents(t, events.NameAuditEntry) {
		if e := ev.(*events.AuditEntry); e.TargetType == "contact" {
			contactEntries = append(contactEntries, e)
		}
	}
	require.Len(t, contactEntries, 3)
	assert.Equal(t, "contact.create", contactEntries[0].Action)
	assert.Equal(t, "contact.update", contactEntries[1].Action)
	assert.Equal(t, "contact.delete", contactEntries[2].Action)
	assert.Equal(t, "changed", contactEntries[0].Diff["email"])
	assert.Equal(t, "changed", contactEntries[0].Diff["first_name"])
	assert.Equal(t, map[string]any{"last_name": "changed", "custom_fields": "changed"}, contactEntries[1].Diff)

	for _, e := range contactEntries {
		assert.Equal(t, string(id), e.TargetID)
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
