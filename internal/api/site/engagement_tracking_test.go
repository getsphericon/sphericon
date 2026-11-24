package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func trackingParams(contactID int64) siteapi.SiteEngagementTrackingGetParams {
	return siteapi.SiteEngagementTrackingGetParams{WorkspaceId: fixtures.AcmePublicID, ContactId: siteapi.EntityId(contactID)}
}

func trackingRecordParams(contactID int64) siteapi.SiteEngagementTrackingCreateParams {
	return siteapi.SiteEngagementTrackingCreateParams{WorkspaceId: fixtures.AcmePublicID, ContactId: siteapi.EntityId(contactID)}
}

// trackingOfEmail is a member's observation of an email address's Engagement tracking consent.
func trackingOfEmail(address string, state siteapi.EngagementTrackingState) *siteapi.SiteRecordEngagementTrackingInput {
	return &siteapi.SiteRecordEngagementTrackingInput{
		Channel: siteapi.EngagementTrackingChannelEmail, Destination: address, State: state,
	}
}

// A member reads on the Contact card that its email address follows the default, then records the
// refusal the person made by phone: the address reads denied, recorded by a member, and the
// evidentiary Event is recorded (ADR 0120).
func TestAMemberRecordsARefusalOfTrackingOnTheContactCard(t *testing.T) {
	env := testhelper.Setup(t)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	ctx := t.Context()

	before, err := member.SiteEngagementTrackingGet(ctx, trackingParams(fixtures.ContactAliceID))
	require.NoError(t, err)
	recorded, err := member.SiteEngagementTrackingCreate(ctx, trackingOfEmail(fixtures.ContactAliceEmail, siteapi.EngagementTrackingStateDenied), trackingRecordParams(fixtures.ContactAliceID))
	require.NoError(t, err)
	after, err := member.SiteEngagementTrackingGet(ctx, trackingParams(fixtures.ContactAliceID))
	require.NoError(t, err)

	require.Len(t, before.Destinations, 1)
	assert.Equal(t, fixtures.ContactAliceEmail, before.Destinations[0].Destination)
	assert.Equal(t, siteapi.EngagementTrackingReadingDefault, before.Destinations[0].State)
	assert.Equal(t, siteapi.EngagementTrackingDecisionTrack, before.Destinations[0].Effective, "the default tracks")
	assert.Equal(t, siteapi.EngagementTrackingOutcomeApplied, recorded.Outcome)
	require.Len(t, after.Destinations, 1)
	assert.Equal(t, siteapi.EngagementTrackingReadingDenied, after.Destinations[0].State)
	assert.Equal(t, siteapi.EngagementTrackingProvenanceManual, after.Destinations[0].Provenance.Value)
	assert.Equal(t, siteapi.EngagementTrackingDecisionDontTrack, after.Destinations[0].Effective)
	denied := env.QueuedCount(t, systemevent.EmailEngagementTrackingDenied, map[string]any{
		"destination": fixtures.ContactAliceEmail, "contactId": fixtures.ContactAliceID, "provenance": "manual",
	})
	assert.Equal(t, 1, denied)
}

// A member's grant applies where nothing stands; over a standing refusal it is a conflict, since
// only the person can lift a refusal (ADR 0121), and the refusal stays.
func TestAMembersGrantAppliesWhereNothingStandsAndIsAConflictOverARefusal(t *testing.T) {
	env := testhelper.Setup(t)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	ctx := t.Context()

	granted, err := member.SiteEngagementTrackingCreate(ctx, trackingOfEmail(fixtures.ContactBobEmail, siteapi.EngagementTrackingStateGranted), trackingRecordParams(fixtures.ContactBobID))
	require.NoError(t, err)
	_, err = member.SiteEngagementTrackingCreate(ctx, trackingOfEmail(fixtures.ContactAliceEmail, siteapi.EngagementTrackingStateDenied), trackingRecordParams(fixtures.ContactAliceID))
	require.NoError(t, err)
	_, regrant := member.SiteEngagementTrackingCreate(ctx, trackingOfEmail(fixtures.ContactAliceEmail, siteapi.EngagementTrackingStateGranted), trackingRecordParams(fixtures.ContactAliceID))
	read, err := member.SiteEngagementTrackingGet(ctx, trackingParams(fixtures.ContactAliceID))
	require.NoError(t, err)

	assert.Equal(t, siteapi.EngagementTrackingReadingGranted, granted.Consent.State)
	assert.Equal(t, siteapi.EngagementTrackingProvenanceManual, granted.Consent.Provenance.Value)
	assert.Equal(t, siteapi.EngagementTrackingDecisionTrack, granted.Consent.Effective)
	refused := testhelper.SiteRefusal(t, regrant, http.StatusConflict)
	assert.Contains(t, refused.Detail.Value, "only the person can lift a refusal")
	require.Len(t, read.Destinations, 1)
	assert.Equal(t, siteapi.EngagementTrackingReadingDenied, read.Destinations[0].State)
}

// The card records the consent of the Contact's own Destinations only, and a Contact of another
// Workspace is not found.
func TestTrackingConsentOnTheCardIsTheContactsOwnAndNotFoundInAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()

	_, notHers := owner.SiteEngagementTrackingCreate(ctx, trackingOfEmail(fixtures.ContactBobEmail, siteapi.EngagementTrackingStateDenied), trackingRecordParams(fixtures.ContactAliceID))
	_, foreignRead := owner.SiteEngagementTrackingGet(ctx, trackingParams(fixtures.TyrellMatrixConfirmedID))
	_, foreignWrite := owner.SiteEngagementTrackingCreate(ctx, trackingOfEmail(fixtures.ContactAliceEmail, siteapi.EngagementTrackingStateDenied), trackingRecordParams(fixtures.TyrellMatrixConfirmedID))
	bob, err := owner.SiteEngagementTrackingGet(ctx, trackingParams(fixtures.ContactBobID))
	require.NoError(t, err)

	testhelper.SiteRefusal(t, notHers, http.StatusUnprocessableEntity)
	testhelper.SiteRefusal(t, foreignRead, http.StatusNotFound)
	testhelper.SiteRefusal(t, foreignWrite, http.StatusNotFound)
	require.Len(t, bob.Destinations, 1)
	assert.Equal(t, siteapi.EngagementTrackingReadingDefault, bob.Destinations[0].State, "Bob's address was not touched")
}
