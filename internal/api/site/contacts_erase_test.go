package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Erasure is irreversible, so a plain member is refused and nothing is touched.
func TestSiteContactsEraseIsRefusedForMembers(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteActor(t, fixtures.MemberMaryEmail)

	res, err := c.SiteContactsDelete(ctx, siteapi.SiteContactsDeleteParams{Slug: fixtures.AcmeSlug, ID: idStr(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsDeleteForbidden{}, res)

	still, err := env.DB.Contact.Query().Where(contact.ID(fixtures.ContactBobID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, still)
}

// An owner's delete is full Erasure: the Contact is gone and its delivery rows are
// anonymized, not left behind.
func TestSiteContactsDeleteByOwnerErases(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteContactsDelete(ctx, siteapi.SiteContactsDeleteParams{Slug: fixtures.AcmeSlug, ID: idStr(fixtures.ContactLiamID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsDeleteNoContent{}, res)

	msg, err := env.DB.OutboundMessage.Get(ctx, 100)
	require.NoError(t, err)
	assert.Empty(t, msg.Destination, "the delivery record no longer carries the address")
	assert.Nil(t, msg.ContactID)
}

// The contact.erased signal names the User who erased.
func TestSiteContactsEraseRecordsTheUserStaffMember(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteContactsDelete(context.Background(), siteapi.SiteContactsDeleteParams{Slug: fixtures.AcmeSlug, ID: idStr(fixtures.ContactLiamID)})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteContactsDeleteNoContent{}, res)

	msgs := env.Outbox(t, events.NameContactErased)
	require.Len(t, msgs, 1)
	assert.Equal(t, "user", msgs[0].Data["staffKind"])
	assert.NotZero(t, msgs[0].Data["staffId"])
}
