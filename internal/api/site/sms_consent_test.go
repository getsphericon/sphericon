package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func smsParams(contactID int64) siteapi.SiteSmsConsentsGetParams {
	return siteapi.SiteSmsConsentsGetParams{WorkspaceId: fixtures.AcmePublicID, ContactId: siteapi.EntityId(contactID)}
}

// A member records, in the app, the consent a Contact's phone gave elsewhere and then an opt-out
// that reached them off-channel; the Contact card reads each (ADR 0066, 0077).
func TestAMemberRecordsSMSConsentAndAnOptOutForAContactsPhone(t *testing.T) {
	env := testhelper.Setup(t)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	ctx := t.Context()

	before, err := member.SiteSmsConsentsGet(ctx, smsParams(fixtures.ContactTexterID))
	require.NoError(t, err)
	confirmed, err := member.SiteSmsConsentsConfirm(ctx, &siteapi.ConsentEvidence{
		Wording: "Text me about orders and offers", Source: "phone call", Brand: "Acme",
	}, siteapi.SiteSmsConsentsConfirmParams{WorkspaceId: fixtures.AcmePublicID, ContactId: siteapi.EntityId(fixtures.ContactTexterID)})
	require.NoError(t, err)
	eligible, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelSMS, fixtures.ContactTexterPhone, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	stopped, err := member.SiteSmsConsentsOptOut(ctx, siteapi.SiteSmsConsentsOptOutParams{WorkspaceId: fixtures.AcmePublicID, ContactId: siteapi.EntityId(fixtures.ContactTexterID)})
	require.NoError(t, err)

	assert.Equal(t, fixtures.ContactTexterPhone, before.Phone.Value)
	assert.False(t, before.Confirmation.Set, "no consent yet")
	assert.False(t, before.OptOut.Set, "no opt-out yet")

	after := confirmed
	assert.Equal(t, siteapi.ConfirmationProvenanceImported, after.Confirmation.Value.Provenance)
	assert.Equal(t, "phone call", after.Confirmation.Value.Source.Value)
	assert.True(t, eligible.Eligible)
	smsConfirmed := env.QueuedCount(t, events.NameSmsConfirmed, map[string]any{
		"phone": fixtures.ContactTexterPhone, "contactId": fixtures.ContactTexterID, "source": "phone call",
	})
	assert.Equal(t, 1, smsConfirmed)
	marketingConfirmed := env.QueuedCount(t, events.NameMarketingConfirmed, nil)
	assert.Equal(t, 0, marketingConfirmed, "marketing.confirmed is email's")

	require.True(t, stopped.OptOut.Set, "opted out")
	assert.Equal(t, siteapi.OptOutViaManual, stopped.OptOut.Value.Via, "recorded by hand: an off-channel opt-out")
	assert.False(t, stopped.Confirmation.Set, "the opt-out drops the stale consent")
}

func TestSMSConsentIsRefusedForAContactWithoutAPhoneAndNotFoundInAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()

	_, noPhone := owner.SiteSmsConsentsOptOut(ctx, siteapi.SiteSmsConsentsOptOutParams{WorkspaceId: fixtures.AcmePublicID, ContactId: siteapi.EntityId(fixtures.ContactAliceID)})
	_, foreign := owner.SiteSmsConsentsGet(ctx, smsParams(fixtures.TyrellMatrixConfirmedID))
	none, err := owner.SiteSmsConsentsGet(ctx, smsParams(fixtures.ContactAliceID))
	require.NoError(t, err)

	testhelper.SiteRefusal(t, noPhone, http.StatusUnprocessableEntity)
	testhelper.SiteRefusal(t, foreign, http.StatusNotFound)
	assert.False(t, none.Phone.Set, "no phone")
}
