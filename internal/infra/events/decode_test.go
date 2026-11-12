package events_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

func TestDecodeKnowsEveryEventName(t *testing.T) {
	for _, name := range []string{
		systemevent.ContactCreated,
		systemevent.ContactErased,
		systemevent.EmailSent,
		systemevent.EmailDelivered,
		systemevent.EmailDelayed,
		systemevent.EmailDropped,
		systemevent.EmailOpened,
		systemevent.EmailClicked,
		systemevent.EmailUnsubscribed,
		systemevent.EmailBounced,
		systemevent.EmailComplained,
		systemevent.MarketingConfirmed,
		systemevent.SmsConfirmed,
		systemevent.SmsDelivered,
		systemevent.SmsUndelivered,
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
		&events.EmailEngagement{Action: systemevent.EmailOpened, WorkspaceID: 7},
		&events.EmailDelivery{Action: systemevent.EmailBounced, WorkspaceID: 7},
		&events.MarketingConfirmed{WorkspaceID: 7},
		&events.SmsConfirmed{WorkspaceID: 7},
		&events.SmsDelivery{Action: systemevent.SmsUndelivered, WorkspaceID: 7},
		&events.CollectedEvent{WorkspaceID: 7, Action: "page_view"},
	} {
		assert.EqualValues(t, 7, ev.Workspace(), "%T", ev)
		assert.Positive(t, ev.EventVersion(), "%T", ev)
		assert.NotEmpty(t, ev.EventName(), "%T", ev)
		assert.NotEmpty(t, ev.Project().Action, "%T", ev)
	}
}
