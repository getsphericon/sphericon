package site_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/capturedmessage"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The address of fixtures' CapturedMessageGlobexWelcome: no Contact and no delivery record holds it.
const capturedOnlyAddress = "ada@globex.test"

// A Privacy request lookup counts the Captured messages sent to an address, also those kept by an
// archived Test inbox, and carries no content (the holdings are counts).
func TestSiteContactsLookupCountsTheCapturedMessagesOfAnAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res := lookup(t, c, fixtures.AcmePublicID, fixtures.ContactExportSubjectEmail)

	got, ok := res.(*siteapi.SiteContactHoldings)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, int32(1), got.CapturedMessages)
}

// An address with no Contact and no delivery record, held by a Captured message alone, is still
// reported as held.
func TestSiteContactsLookupAddressHeldOnlyByACapturedMessage(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	res := lookup(t, c, fixtures.GlobexPublicID, capturedOnlyAddress)

	got, ok := res.(*siteapi.SiteContactHoldings)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.SiteContactHoldings{CapturedMessages: 1}, *got)
}

// An address held only by a Captured message can be erased, and the erasure takes that message and
// no other.
func TestSiteContactsEraseByEmailDeletesAnAddressHeldOnlyByACapturedMessage(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	res := eraseByEmail(t, c, fixtures.GlobexPublicID, capturedOnlyAddress)

	require.IsType(t, &siteapi.SiteContactsEraseNoContent{}, res)
	gone := env.DB.CapturedMessage.Query().Where(capturedmessage.ID(fixtures.CapturedMessageGlobexWelcomeID)).ExistX(t.Context())
	assert.False(t, gone)
	kept := env.DB.CapturedMessage.Query().Where(capturedmessage.ID(fixtures.CapturedMessageGlobexNewsletterID)).ExistX(t.Context())
	assert.True(t, kept, "another address's message stays")
	got, ok := lookup(t, c, fixtures.GlobexPublicID, capturedOnlyAddress).(*siteapi.SiteContactHoldings)
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteContactHoldings{}, *got)
}

// Deleting a Contact is Erasure: the Captured messages addressed to its Destinations go with it, also
// those kept by an archived Test inbox, and another Workspace's messages stay.
func TestSiteContactsDeleteErasesTheCapturedMessagesOfTheContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	globex := env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(t.Context())
	require.NotZero(t, env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.AcmeID)).CountX(t.Context()))

	res, err := c.SiteContactsDelete(t.Context(), siteapi.SiteContactsDeleteParams{
		WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.ContactExportSubjectID),
	})

	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteContactsDeleteNoContent{}, res)
	assert.Zero(t, env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.AcmeID)).CountX(t.Context()))
	assert.Equal(t, globex, env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(t.Context()))
}

// Archiving a Test inbox stops capturing and keeps what it captured until it expires; only a
// Workspace deletion removes it sooner.
func TestArchivingATestInboxKeepsItsCapturedMessages(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	out, err := c.SiteIntegrationsArchive(t.Context(), siteapi.SiteIntegrationsArchiveParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.IntegrationGlobexInboxID),
	})

	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteIntegrationResource{}, out)
	assert.Equal(t, 2, env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(t.Context()))
}
