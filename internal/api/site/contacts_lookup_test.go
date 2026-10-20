package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func lookup(t *testing.T, c *siteapi.Client, slug, email string) siteapi.SiteContactsLookupRes {
	t.Helper()
	res, err := c.SiteContactsLookup(context.Background(), siteapi.SiteContactsLookupParams{
		Slug:  slug,
		Email: email,
	})
	require.NoError(t, err)
	return res
}

// A Contact's address answers with what the Workspace holds for it, as counts.
func TestSiteContactsLookupCountsWhatIsHeldForAContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res := lookup(t, c, fixtures.AcmeSlug, fixtures.ContactExportSubjectEmail)

	got, ok := res.(*siteapi.SiteContactLookup)
	require.Truef(t, ok, "got %T", res)
	assert.True(t, got.ContactHeld)
	assert.Equal(t, int32(3), got.Events)
	assert.Equal(t, int32(2), got.DeliveryRecords)
	assert.Equal(t, int32(1), got.Unsubscribes)
	assert.Equal(t, int32(1), got.Suppressions)
}

// The address is lowercased, like the External API does.
func TestSiteContactsLookupNormalizesTheAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res := lookup(t, c, fixtures.AcmeSlug, "Export.Subject@Example.com")

	got, ok := res.(*siteapi.SiteContactLookup)
	require.Truef(t, ok, "got %T", res)
	assert.True(t, got.ContactHeld)
}

// An address with delivery records and no Contact is held by those records alone.
func TestSiteContactsLookupAddressWithoutContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)

	res := lookup(t, c, fixtures.InitechSlug, fixtures.OutboundMessageStrangerDestination)

	got, ok := res.(*siteapi.SiteContactLookup)
	require.Truef(t, ok, "got %T", res)
	assert.False(t, got.ContactHeld)
	assert.Equal(t, int32(1), got.DeliveryRecords)
	assert.Equal(t, int32(0), got.Events)
}

// Nothing held is a valid answer: zeros, not 404.
func TestSiteContactsLookupUnknownAddressIsZeros(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res := lookup(t, c, fixtures.AcmeSlug, "nobody@nowhere.test")

	got, ok := res.(*siteapi.SiteContactLookup)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.SiteContactLookup{}, *got)
}

// Another Workspace's data is never matched.
func TestSiteContactsLookupNeverMatchesAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res := lookup(t, c, fixtures.AcmeSlug, "pat@globex.test")

	got, ok := res.(*siteapi.SiteContactLookup)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.SiteContactLookup{}, *got)
}

// An admin may look up; a plain member is refused.
func TestSiteContactsLookupIsOwnerAndAdminOnly(t *testing.T) {
	env := testhelper.Setup(t)

	admin := env.SiteActor(t, fixtures.InitechAdminAdaEmail)
	res := lookup(t, admin, fixtures.InitechSlug, "erin@initech.test")
	assert.IsType(t, &siteapi.SiteContactLookup{}, res)

	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	res = lookup(t, member, fixtures.AcmeSlug, fixtures.ContactExportSubjectEmail)
	assert.IsType(t, &siteapi.SiteContactsLookupForbidden{}, res)
}

// An invalid address is rejected before any lookup.
func TestSiteContactsLookupRejectsAnInvalidAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res := lookup(t, c, fixtures.AcmeSlug, "not an address")

	assert.IsType(t, &siteapi.SiteContactsLookupBadRequest{}, res)
}

// A padded, mixed-case address is trimmed and lowercased, not refused by the
// generated format check (Privacy request, story 20).
func TestSiteContactsLookupAcceptsAPaddedAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res := lookup(t, c, fixtures.AcmeSlug, " Export.Subject@Example.com ")

	got, ok := res.(*siteapi.SiteContactLookup)
	require.Truef(t, ok, "got %T", res)
	assert.True(t, got.ContactHeld)
}
