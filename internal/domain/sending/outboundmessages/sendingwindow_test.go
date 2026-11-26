package outboundmessages_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Sending window (ADR 0134) at the Outbound send seam: a marketing text goes out only while the
// Workspace's window is open in every zone the recipient may be in, and waits for it otherwise. The
// fixture windows are open all day: each test narrows Lacuna's to its hours. Lacuna's texters have
// Boston numbers, in New York's zone alone. The Instance runs on the test's clock, set to a moment
// after the real one, so the fixtures' times keep their meaning.

// windowTest is a test of Lacuna's Sending window on the Instance's clock.
type windowTest struct {
	env     *testhelper.TestEnv
	clock   *testhelper.Clock
	account testhelper.TwilioAccount
}

func newWindowTest(t *testing.T, at time.Time) *windowTest {
	t.Helper()
	c := testhelper.NewClock(at)
	env := testhelper.Setup(t, testhelper.WithClock(c.Now))
	return &windowTest{env: env, clock: c, account: env.LacunaTwilio(t)}
}

func lacuna(env *testhelper.TestEnv) *ent.Scoped { return env.DB.Scoped(fixtures.LacunaID) }

// newYork is the zone of Lacuna's texters' numbers.
var newYork = lo.Must(time.LoadLocation("America/New_York"))

// tomorrowAt is hour o'clock in loc on the day after today there: a moment after the real one.
func tomorrowAt(loc *time.Location, hour int) time.Time {
	return sameDayAt(time.Now().In(loc).AddDate(0, 0, 1), hour)
}

// sameDayAt is hour o'clock on the day t reads in its location.
func sameDayAt(t time.Time, hour int) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, t.Location())
}

// text sends Lacuna's Broadcast text under key to the Contact's number now, and requires no error.
func (wt *windowTest) text(t *testing.T, contactID int64, key string) outboundmessages.Result {
	t.Helper()
	c := wt.env.DB.Contact.GetX(t.Context(), contactID)
	id := int64(wt.account.Integration.ID)
	res, err := wt.env.OutboundModule().Send(t.Context(), lacuna(wt.env), outboundmessages.Request{
		Ref: outboundmessages.BroadcastRef{BroadcastID: fixtures.BroadcastLacunaSmsHelloID}, Key: key,
		Destination: lo.FromPtr(c.Phone), Contact: c, Content: outboundmessages.SMSContent{Text: "Sale"}, IntegrationID: &id,
	})
	require.NoError(t, err)
	return res
}

// message is Lacuna's Outbound message under key, nil when none is recorded.
func (wt *windowTest) message(t *testing.T, key string) *ent.OutboundMessage {
	t.Helper()
	msg, err := wt.env.DB.OutboundMessage.Query().
		Where(outboundmessage.WorkspaceID(fixtures.LacunaID), outboundmessage.IdempotencyKey(key)).Only(t.Context())
	if ent.IsNotFound(err) {
		return nil
	}
	require.NoError(t, err)
	return msg
}

// readsAt reports whether t reads as hour:00 in loc.
func readsAt(t time.Time, loc *time.Location, hour int) bool {
	local := t.In(loc)
	return local.Hour() == hour && local.Minute() == 0 && local.Second() == 0
}

// Ada's number got Lacuna's cap of three texts a day at 7:00, while its window was still open all
// day; then the window became 8:00 to 20:00. A text at 22:00 is not read against the cap: the
// window, read first, defers it until 8:00 the next day, and nothing is recorded. By 8:00 the
// earlier texts have aged out of the cap, and it is sent.
func TestATextAt22InItsZoneIsDeferredUntil8ThereAndThenSentUnderTheCap(t *testing.T) {
	early := tomorrowAt(newYork, 7)
	wt := newWindowTest(t, early)
	for i := range 3 {
		require.Equal(t, outboundmessages.Sent, wt.text(t, fixtures.ContactLacunaAdaID, fmt.Sprintf("earlier:%d", i)).Outcome)
	}
	wt.env.WindowSMS(t, fixtures.LacunaID, 8*time.Hour, 20*time.Hour)
	night := sameDayAt(early, 22)
	wt.clock.Set(night)

	waits := wt.text(t, fixtures.ContactLacunaAdaID, "window:night")

	assert.Equal(t, outboundmessages.Deferral, waits.Outcome)
	assert.Equal(t, deferralcause.SendingWindow, waits.Cause)
	assert.True(t, readsAt(waits.Until, newYork, 8), "until 8:00 in New York: %s", waits.Until.In(newYork))
	assert.True(t, waits.Until.After(night))
	assert.Less(t, waits.Until.Sub(night), 12*time.Hour, "the next morning")
	assert.Nil(t, wt.message(t, "window:night"), "a Deferral records nothing")
	assert.Len(t, wt.env.Twilio.Messages(), 3)

	wt.clock.Set(waits.Until.Add(5 * time.Minute))
	sent := wt.text(t, fixtures.ContactLacunaAdaID, "window:night")

	assert.Equal(t, outboundmessages.Sent, sent.Outcome)
	assert.Len(t, wt.env.Twilio.Messages(), 4)
	msg := wt.message(t, "window:night")
	require.NotNil(t, msg)
	assert.Equal(t, outboundmessage.WindowZonesOneZone, msg.WindowZones, "a Boston number is judged by New York alone")
}

// Ada texted STOP overnight: when her window opens, the text that waited is read again from the top
// and Skipped for her opt-out, not sent.
func TestATextThatWaitedForTheWindowIsSkippedForASTOPReceivedOvernight(t *testing.T) {
	night := tomorrowAt(newYork, 22)
	wt := newWindowTest(t, night)
	wt.env.WindowSMS(t, fixtures.LacunaID, 8*time.Hour, 20*time.Hour)
	waits := wt.text(t, fixtures.ContactLacunaAdaID, "window:stop")
	require.Equal(t, outboundmessages.Deferral, waits.Outcome)
	wt.env.DB.Unsubscribe.UpdateOneID(fixtures.UnsubscribeLacunaParkedID).SetDestination(fixtures.ContactLacunaAdaPhone).ExecX(t.Context())

	wt.clock.Set(waits.Until)
	res := wt.text(t, fixtures.ContactLacunaAdaID, "window:stop")

	assert.Equal(t, outboundmessages.Skipped, res.Outcome)
	assert.Equal(t, skipreason.UnsubscribedEverything, res.SkippedReason)
	assert.Empty(t, wt.env.Twilio.Messages())
}

// tollFreeAda moves Ada and her Confirmation onto a US toll-free number: a prefix the phone number
// library maps to no zone of its own.
func (wt *windowTest) tollFreeAda(t *testing.T) {
	t.Helper()
	const tollFree = "+18005550100"
	wt.env.DB.Contact.UpdateOneID(fixtures.ContactLacunaAdaID).SetPhone(tollFree).ExecX(t.Context())
	wt.env.DB.Confirmation.UpdateOneID(fixtures.ConfirmationLacunaAdaID).SetDestination(tollFree).ExecX(t.Context())
}

var honolulu = lo.Must(time.LoadLocation("Pacific/Honolulu"))

// A toll-free number is judged by every zone of the United States, Honolulu's included: at 9:00 in
// New York it is night there, so the text waits until 8:00 in Honolulu.
func TestATextToANumberWhosePrefixMapsNoZoneWaitsForEveryZoneOfItsCountry(t *testing.T) {
	wt := newWindowTest(t, tomorrowAt(newYork, 9))
	wt.env.WindowSMS(t, fixtures.LacunaID, 8*time.Hour, 20*time.Hour)
	wt.tollFreeAda(t)

	res := wt.text(t, fixtures.ContactLacunaAdaID, "window:toll-free")

	assert.Equal(t, outboundmessages.Deferral, res.Outcome)
	assert.True(t, readsAt(res.Until, honolulu, 8), "until 8:00 in Honolulu: %s", res.Until.In(honolulu))
}

// From 9:00 to 12:00, New York and Honolulu are never open at once: a toll-free number has no local
// time, and its text is Skipped with the reason, recorded under its key.
func TestATextToANumberWhoseZonesNeverShareAnOpenMomentIsSkippedWithNoLocalTime(t *testing.T) {
	wt := newWindowTest(t, tomorrowAt(newYork, 10))
	wt.env.WindowSMS(t, fixtures.LacunaID, 9*time.Hour, 12*time.Hour)
	wt.tollFreeAda(t)

	res := wt.text(t, fixtures.ContactLacunaAdaID, "window:nowhere")

	assert.Equal(t, outboundmessages.Skipped, res.Outcome)
	assert.Equal(t, skipreason.NoLocalTime, res.SkippedReason)
	msg := wt.message(t, "window:nowhere")
	require.NotNil(t, msg)
	assert.Equal(t, skipreason.NoLocalTime, lo.FromPtr(msg.SkippedReason))
	assert.Equal(t, outboundmessage.WindowZonesSeveralZones, msg.WindowZones)
	assert.Empty(t, wt.env.Twilio.Messages())
}

// globalAda moves Ada and her Confirmation onto a global service number: of no country, so in no
// zone of its own.
func (wt *windowTest) globalAda(t *testing.T) {
	t.Helper()
	const global = "+80012345678"
	wt.env.DB.Contact.UpdateOneID(fixtures.ContactLacunaAdaID).SetPhone(global).ExecX(t.Context())
	wt.env.DB.Confirmation.UpdateOneID(fixtures.ConfirmationLacunaAdaID).SetDestination(global).ExecX(t.Context())
}

// Ada's global service number is in no zone, and her time zone says New York: the window is judged
// there alone, so at 22:00 in New York her text waits until 8:00 there, and then goes, judged by
// one zone.
func TestATextToANumberInNoZoneIsJudgedByTheContactsZone(t *testing.T) {
	wt := newWindowTest(t, tomorrowAt(newYork, 22))
	wt.env.WindowSMS(t, fixtures.LacunaID, 8*time.Hour, 20*time.Hour)
	wt.globalAda(t)
	wt.env.DB.Contact.UpdateOneID(fixtures.ContactLacunaAdaID).SetTimeZone("America/New_York").ExecX(t.Context())

	waits := wt.text(t, fixtures.ContactLacunaAdaID, "window:global")
	wt.clock.Set(waits.Until)
	sent := wt.text(t, fixtures.ContactLacunaAdaID, "window:global")

	assert.Equal(t, outboundmessages.Deferral, waits.Outcome)
	assert.True(t, readsAt(waits.Until, newYork, 8), "until 8:00 in New York: %s", waits.Until.In(newYork))
	assert.Equal(t, outboundmessages.Sent, sent.Outcome)
	msg := wt.message(t, "window:global")
	require.NotNil(t, msg)
	assert.Equal(t, outboundmessage.WindowZonesOneZone, msg.WindowZones)
}

// Without a time zone of hers, Ada's global service number is in no zone at all: her text has no
// local time, and its message says no zone judged it.
func TestATextToANumberInNoZoneOfAContactWithoutOneIsSkippedWithNoLocalTime(t *testing.T) {
	wt := newWindowTest(t, tomorrowAt(newYork, 10))
	wt.env.WindowSMS(t, fixtures.LacunaID, 8*time.Hour, 20*time.Hour)
	wt.globalAda(t)

	res := wt.text(t, fixtures.ContactLacunaAdaID, "window:global-alone")

	assert.Equal(t, outboundmessages.Skipped, res.Outcome)
	assert.Equal(t, skipreason.NoLocalTime, res.SkippedReason)
	msg := wt.message(t, "window:global-alone")
	require.NotNil(t, msg)
	assert.Equal(t, outboundmessage.WindowZonesNoZone, msg.WindowZones)
}

// At 22:00 in New York it is noon in Tokyo: a Contact whose time zone says Tokyo does not open the
// window her Boston number keeps closed. Her text waits for a moment open in both: 8:00 in Tokyo.
func TestAContactsTimeZoneThatIsOpenWhereHerNumberIsClosedDoesNotSendThen(t *testing.T) {
	wt := newWindowTest(t, tomorrowAt(newYork, 22))
	wt.env.WindowSMS(t, fixtures.LacunaID, 8*time.Hour, 20*time.Hour)
	wt.env.DB.Contact.UpdateOneID(fixtures.ContactLacunaAdaID).SetTimeZone("Asia/Tokyo").ExecX(t.Context())

	res := wt.text(t, fixtures.ContactLacunaAdaID, "window:tokyo")

	assert.Equal(t, outboundmessages.Deferral, res.Outcome)
	tokyo := lo.Must(time.LoadLocation("Asia/Tokyo"))
	assert.True(t, readsAt(res.Until, tokyo, 8), "until 8:00 in Tokyo: %s", res.Until.In(tokyo))
	assert.Empty(t, wt.env.Twilio.Messages())
}

// A time zone the zone database does not know counts for nothing: at 10:00 in New York the text
// goes.
func TestAContactsTimeZoneThatIsNoZoneIsIgnored(t *testing.T) {
	wt := newWindowTest(t, tomorrowAt(newYork, 10))
	wt.env.WindowSMS(t, fixtures.LacunaID, 8*time.Hour, 20*time.Hour)
	wt.env.DB.Contact.UpdateOneID(fixtures.ContactLacunaAdaID).SetTimeZone("Mars/Olympus_Mons").ExecX(t.Context())

	res := wt.text(t, fixtures.ContactLacunaAdaID, "window:mars")

	assert.Equal(t, outboundmessages.Sent, res.Outcome)
}

// A Test send goes to the number its author types: no window holds it, at any hour.
func TestATestSendAtNightGoesOut(t *testing.T) {
	wt := newWindowTest(t, tomorrowAt(newYork, 3))
	wt.env.WindowSMS(t, fixtures.LacunaID, 8*time.Hour, 20*time.Hour)
	b := wt.env.DB.Broadcast.GetX(t.Context(), fixtures.BroadcastLacunaSmsHelloID)

	res, err := wt.env.OutboundModule().SendBroadcastTest(t.Context(), lacuna(wt.env), b, outboundmessages.TestAddress{Phone: fixtures.ContactLacunaAdaPhone})

	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome)
	assert.Len(t, wt.env.Twilio.Messages(), 1)
}

// No law sets hours for email: a marketing email at 3:00 goes, whatever the texts' window.
func TestAMarketingEmailAtNightGoesOut(t *testing.T) {
	wt := newWindowTest(t, tomorrowAt(newYork, 3))
	wt.env.WindowSMS(t, fixtures.AcmeID, 8*time.Hour, 20*time.Hour)

	res, err := wt.env.OutboundModule().Send(t.Context(), acme(wt.env), marketing(t, wt.env, "window:email", fixtures.ContactAliceID))

	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, res.Outcome)
	msg := byKey(t, wt.env, "window:email")
	assert.Equal(t, outboundmessage.WindowZonesUnjudged, msg.WindowZones, "no window held it")
}
