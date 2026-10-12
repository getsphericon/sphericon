package external_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The same edit through /api is audited under the token, unlike ingest.
func TestExternalTagCreateIsAuditedUnderTheToken(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalAnchor(t)

	res, err := c.TagsApply(context.Background(), &externalapi.ApplyTagInput{Name: "api-tag"},
		externalapi.TagsApplyParams{ContactId: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)
	require.IsType(t, &externalapi.TagResource{}, res)

	got := env.OutboxEvents(t, events.NameAuditEntry)
	require.Len(t, got, 1)
	e := got[0].(*events.AuditEntry)
	assert.Equal(t, "tag.create", e.Action)
	assert.Equal(t, events.Actor{Kind: events.ActorAPIToken, ID: strconv.Itoa(fixtures.AnchorTokenID), Name: fixtures.AnchorTokenName}, e.Actor)
	assert.Equal(t, "api-tag", e.TargetName)

	// The owner reads it on /site, attributed to the token.
	env.DeliverToEE(t)
	list, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteAuditList(context.Background(), siteapi.SiteAuditListParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	page, ok := list.(*siteapi.SiteAuditEntryList)
	require.Truef(t, ok, "got %T", list)
	require.NotEmpty(t, page.Items)
	listed := page.Items[0]
	assert.Equal(t, "tag.create", listed.Action)
	assert.Equal(t, siteapi.SiteAuditActorKindAPIToken, listed.Actor.Kind)
	assert.Equal(t, strconv.Itoa(fixtures.AnchorTokenID), listed.Actor.ID.Value)
}
