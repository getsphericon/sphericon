package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func lookup(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId, email string) *siteapi.SiteContactHoldings {
	t.Helper()
	res, err := c.SiteContactsLookup(context.Background(), siteapi.SiteContactsLookupParams{
		WorkspaceId: workspaceID,
		Email:       email,
	})
	require.NoError(t, err)
	return res
}

// A Contact's address answers with what the Workspace holds for it, as counts.
func TestSiteContactsLookupCountsWhatIsHeldForAContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	got := lookup(t, c, fixtures.AcmePublicID, fixtures.ContactExportSubjectEmail)
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

	got := lookup(t, c, fixtures.AcmePublicID, "Export.Subject@Example.com")
	assert.True(t, got.ContactHeld)
}

// An address with delivery records and no Contact is held by those records alone.
func TestSiteContactsLookupAddressWithoutContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)

	got := lookup(t, c, fixtures.InitechPublicID, fixtures.OutboundMessageStrangerDestination)
	assert.False(t, got.ContactHeld)
	assert.Equal(t, int32(1), got.DeliveryRecords)
	assert.Equal(t, int32(0), got.Events)
}

// Nothing held is a valid answer: zeros, not 404.
func TestSiteContactsLookupUnknownAddressIsZeros(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	got := lookup(t, c, fixtures.AcmePublicID, "nobody@nowhere.test")
	assert.Equal(t, siteapi.SiteContactHoldings{}, *got)
}

// Another Workspace's data is never matched.
func TestSiteContactsLookupNeverMatchesAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	got := lookup(t, c, fixtures.AcmePublicID, "pat@globex.test")
	assert.Equal(t, siteapi.SiteContactHoldings{}, *got)
}

// An admin may look up; a plain member is refused.
func TestSiteContactsLookupIsOwnerAndAdminOnly(t *testing.T) {
	env := testhelper.Setup(t)

	admin := env.SiteActor(t, fixtures.InitechAdminAdaEmail)
	lookup(t, admin, fixtures.InitechPublicID, "erin@initech.test")

	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	_, err := member.SiteContactsLookup(context.Background(), siteapi.SiteContactsLookupParams{
		WorkspaceId: fixtures.AcmePublicID,
		Email:       fixtures.ContactExportSubjectEmail,
	})
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
}

// An invalid address is rejected before any lookup.
func TestSiteContactsLookupRejectsAnInvalidAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteContactsLookup(context.Background(), siteapi.SiteContactsLookupParams{
		WorkspaceId: fixtures.AcmePublicID,
		Email:       "not an address",
	})

	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}

// A padded, mixed-case address is trimmed and lowercased, not refused by the
// generated format check (Privacy request, story 20).
func TestSiteContactsLookupAcceptsAPaddedAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	got := lookup(t, c, fixtures.AcmePublicID, " Export.Subject@Example.com ")
	assert.True(t, got.ContactHeld)
}
