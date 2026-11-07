package events_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/events"
)

func TestDecodeKnowsEveryEventName(t *testing.T) {
	for _, name := range []string{
		events.NameContactCreated,
		events.NameContactErased,
		events.NameEmailSent,
		events.NameEmailDelivered,
		events.NameEmailDelayed,
		events.NameEmailDropped,
		events.NameEmailOpened,
		events.NameEmailClicked,
		events.NameEmailUnsubscribed,
		events.NameEmailBounced,
		events.NameEmailComplained,
		events.NameMarketingConfirmed,
		events.NameSmsConfirmed,
		events.NameSmsDelivered,
		events.NameSmsUndelivered,
		events.NameCollected,
		events.NameAuditEntry,
		events.NameTrackingDomainChanged,
		events.NameUserVerificationRequested,
		events.NameUserPasswordResetRequested,
		events.NameUserEmailChangeRequested,
		events.NameUserDeletionRequested,
		events.NameUserAddressClaimed,
		events.NameUserSignedUp,
		events.NameUserIdentityLinked,
		events.NameUserDeleted,
		events.NameUserPasskeyAdded,
		events.NameUserPasskeyRemoved,
		events.NameInvitationCreated,
		events.NameWorkspaceSecondFactorRequired,
		events.NameMembershipSecondFactorReminderDue,
		events.NameWorkspaceSuspended,
		events.NameWorkspaceAbuseWarned,
		events.NameJoinDomainLost,
		events.NameSendingDomainVerificationLost,
		events.NameTrackingDomainReplaced,
		events.NameTrackingDomainRemoved,
		events.NameWebhookEndpointDisabled,
	} {
		ev, err := events.Decode(events.Envelope{Name: name, Data: []byte(`{"action":"` + name + `"}`)})
		require.NoError(t, err, name)
		assert.Equal(t, name, ev.EventName(), "an event reports the name it is registered under")
	}
}

func TestEventsReportTheirEnvelopeMetadata(t *testing.T) {
	for _, ev := range []events.Projected{
		&events.ContactCreated{WorkspaceID: 7},
		&events.EmailEngagement{Action: events.NameEmailOpened, WorkspaceID: 7},
		&events.EmailDelivery{Action: events.NameEmailBounced, WorkspaceID: 7},
		&events.MarketingConfirmed{WorkspaceID: 7},
		&events.SmsConfirmed{WorkspaceID: 7},
		&events.SmsDelivery{Action: events.NameSmsUndelivered, WorkspaceID: 7},
		&events.CollectedEvent{WorkspaceID: 7, Action: "page_view"},
	} {
		assert.EqualValues(t, 7, ev.Workspace(), "%T", ev)
		assert.Positive(t, ev.EventVersion(), "%T", ev)
		assert.NotEmpty(t, ev.EventName(), "%T", ev)
		assert.NotEmpty(t, ev.Project().Action, "%T", ev)
	}
}
