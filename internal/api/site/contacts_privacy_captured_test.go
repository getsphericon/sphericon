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

	got := lookup(t, c, fixtures.AcmePublicID, fixtures.ContactExportSubjectEmail)
	assert.Equal(t, int32(1), got.CapturedMessages)
}

// An address with no Contact and no delivery record, held by a Captured message alone, is still
// reported as held.
func TestSiteContactsLookupAddressHeldOnlyByACapturedMessage(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	got := lookup(t, c, fixtures.GlobexPublicID, capturedOnlyAddress)
	assert.Equal(t, siteapi.SiteContactHoldings{CapturedMessages: 1}, *got)
}

// Deleting a Test inbox stops capturing and keeps what it captured until it expires; only a
// Workspace deletion removes it sooner.
func TestDeletingATestInboxKeepsItsCapturedMessages(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	err := c.SiteIntegrationsDelete(t.Context(), siteapi.SiteIntegrationsDeleteParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.IntegrationGlobexInboxID),
	})

	require.NoError(t, err)
	assert.Equal(t, 2, env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(t.Context()))
}
