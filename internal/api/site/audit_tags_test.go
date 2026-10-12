package site_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Applying a new Tag on /site creates it, and the entry names the signed-in User.
func TestSiteTagCreateIsAuditedUnderTheUser(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteTagsApply(context.Background(), &siteapi.SiteApplyTagInput{Name: "audited-tag"},
		siteapi.SiteTagsApplyParams{Slug: fixtures.AcmeSlug, ContactId: idStr(fixtures.ContactBobID)})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteTagResource{}, res)

	got := env.OutboxEvents(t, events.NameAuditEntry)
	require.Len(t, got, 1)
	e := got[0].(*events.AuditEntry)
	assert.Equal(t, "tag.create", e.Action)
	assert.Equal(t, events.Actor{Kind: events.ActorUser, ID: strconv.Itoa(fixtures.OwnerJohnID), Name: "John"}, e.Actor)
	assert.Equal(t, "audited-tag", e.TargetName)
	assert.Equal(t, map[string]any{"name": map[string]any{"to": "audited-tag"}}, e.Diff)

	// Applying it again is idempotent and records nothing more.
	_, err = c.SiteTagsApply(context.Background(), &siteapi.SiteApplyTagInput{Name: "audited-tag"},
		siteapi.SiteTagsApplyParams{Slug: fixtures.AcmeSlug, ContactId: idStr(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.Len(t, env.OutboxEvents(t, events.NameAuditEntry), 1)
}
