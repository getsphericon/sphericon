package channelstop_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/channelstop"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Every new SMS opt-out records sms.unsubscribed (ADR 0113), whatever way the person stopped: for
// the Contact whose phone the number is, and for the Contact of the text the stop came from when
// that is another, which alone is attributed to the text's Broadcast or Automation. ContactTexter
// holds the number below, with neither a Confirmation nor an opt-out.

// stopSMS records st in a transaction of Acme.
func stopSMS(t *testing.T, env *testhelper.TestEnv, st channelstop.SMSStop) {
	t.Helper()
	err := env.Bus.WithinScopedTx(t.Context(), env.DB.Scoped(fixtures.AcmeID), func(ts *ent.Scoped, pub events.Publisher) error {
		return channelstop.StopSMS(t.Context(), ts, pub, st)
	})
	require.NoError(t, err)
}

// unsubscribed is what an sms.unsubscribed says of the opt-out.
type unsubscribed struct {
	ContactID, BroadcastID, AutomationID, IntegrationID int64
	Phone, Via                                          string
}

// recorded are the queued sms.unsubscribed, in the order they were published.
func recorded(t *testing.T, env *testhelper.TestEnv) []unsubscribed {
	t.Helper()
	return lo.Map(env.QueuedEvents(t, events.NameSmsUnsubscribed), func(ev events.DomainEvent, _ int) unsubscribed {
		e := ev.(*events.SmsUnsubscribed)
		return unsubscribed{
			ContactID: e.ContactID, BroadcastID: e.BroadcastID, AutomationID: e.AutomationID,
			IntegrationID: e.IntegrationID, Phone: e.Phone, Via: e.Via,
		}
	})
}

func TestEachWayToStopTextsRecordsSmsUnsubscribedForTheNumbersContact(t *testing.T) {
	vias := []string{
		events.UnsubscribedByKeyword, events.UnsubscribedByLink, events.UnsubscribedByRefusedSend, events.UnsubscribedByManual,
	}
	for _, via := range vias {
		t.Run(via, func(t *testing.T) {
			env := testhelper.Setup(t)

			stopSMS(t, env, channelstop.SMSStop{Number: fixtures.ContactTexterPhone, Via: via, IntegrationID: fixtures.IntegrationAcmeSmsID})

			assert.Equal(t, []unsubscribed{{
				ContactID: fixtures.ContactTexterID, IntegrationID: fixtures.IntegrationAcmeSmsID, Phone: fixtures.ContactTexterPhone, Via: via,
			}}, recorded(t, env))
		})
	}
}

func TestAStopFromATextNamesItsBroadcastAndAutomationOnTheTextsContact(t *testing.T) {
	env := testhelper.Setup(t)

	stopSMS(t, env, channelstop.SMSStop{
		Number: fixtures.ContactTexterPhone, Via: events.UnsubscribedByRefusedSend,
		ContactID: fixtures.ContactTexterID, BroadcastID: fixtures.BroadcastSmsDraftID, AutomationID: fixtures.AutomationWelcomeSeriesID,
	})

	assert.Equal(t, []unsubscribed{{
		ContactID: fixtures.ContactTexterID, BroadcastID: fixtures.BroadcastSmsDraftID, AutomationID: fixtures.AutomationWelcomeSeriesID,
		Phone: fixtures.ContactTexterPhone, Via: events.UnsubscribedByRefusedSend,
	}}, recorded(t, env))
}

// A text went to Alice at a number that is now ContactTexter's: both are told, and only Alice's
// event is the text's.
func TestAStopFromATextOfAnotherContactRecordsAnEventForEachAttributedToTheTextsOnly(t *testing.T) {
	env := testhelper.Setup(t)

	stopSMS(t, env, channelstop.SMSStop{
		Number: fixtures.ContactTexterPhone, Via: events.UnsubscribedByLink,
		ContactID: fixtures.ContactAliceID, BroadcastID: fixtures.BroadcastSmsDraftID,
	})

	assert.Equal(t, []unsubscribed{
		{ContactID: fixtures.ContactAliceID, BroadcastID: fixtures.BroadcastSmsDraftID, Phone: fixtures.ContactTexterPhone, Via: events.UnsubscribedByLink},
		{ContactID: fixtures.ContactTexterID, Phone: fixtures.ContactTexterPhone, Via: events.UnsubscribedByLink},
	}, recorded(t, env))
}

func TestAStopOfANumberNoContactHasRecordsNoEvent(t *testing.T) {
	env := testhelper.Setup(t)

	stopSMS(t, env, channelstop.SMSStop{Number: "+14155550199", Via: events.UnsubscribedByManual})

	assert.Empty(t, recorded(t, env))
}

// A number already opted out records nothing; once a START lifts the opt-out, the next stop is a new
// one and records a new event.
func TestOnlyANewOptOutRecordsSmsUnsubscribed(t *testing.T) {
	env := testhelper.Setup(t)
	stop := channelstop.SMSStop{Number: fixtures.ContactTexterPhone, Via: events.UnsubscribedByKeyword}

	stopSMS(t, env, stop)
	stopSMS(t, env, stop)
	once := len(recorded(t, env))
	err := env.Bus.WithinScopedTx(t.Context(), env.DB.Scoped(fixtures.AcmeID), func(ts *ent.Scoped, pub events.Publisher) error {
		return confirmations.StartSMSIn(t.Context(), ts, pub, fixtures.ContactTexterPhone)
	})
	require.NoError(t, err)
	stopSMS(t, env, stop)
	twice := len(recorded(t, env))

	assert.Equal(t, 1, once, "a repeated stop records nothing")
	assert.Equal(t, 2, twice, "a stop after a START records again")
}
