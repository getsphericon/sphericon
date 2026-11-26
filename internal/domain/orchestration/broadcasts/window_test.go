package broadcasts_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The share of a Broadcast's audience its Sending window would hold at a scheduled moment (ADR 0134).
// Lacuna's texters Ada, Bea, Cyd and Dot have Boston numbers, in New York's zone alone; its
// fixture window is open all day, so each test narrows it to 8:00 to 20:00. 2027-01-15 is a Friday.

var (
	newYork    = lo.Must(time.LoadLocation("America/New_York"))
	losAngeles = lo.Must(time.LoadLocation("America/Los_Angeles"))
)

// narrowLacunaSMS gives Lacuna an SMS window open from 8:00 to 20:00 every day, as its settings would.
func narrowLacunaSMS(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	s := env.DB.Scoped(fixtures.LacunaID)
	m := testhelper.Module[*workspaces.Module](env)
	rules, err := m.SendingRules(t.Context(), s)
	require.NoError(t, err)
	hours := lo.Map(localcalendar.Weekdays, func(day localcalendar.Weekday, _ int) workspaces.OpenHours {
		return workspaces.OpenHours{Day: day, OpensAt: 8 * 60, ClosesAt: 20 * 60}
	})
	_, err = m.ReplaceSendingRules(t.Context(), s, workspaces.ReplaceSendingRulesInput{
		Revision:       rules.Revision,
		FrequencyCaps:  rules.FrequencyCaps,
		SendingWindows: []workspaces.SendingWindow{{Channel: channels.SMS, Hours: hours}},
	})
	require.NoError(t, err)
}

func outsideWindow(t *testing.T, env *testhelper.TestEnv, workspaceID int64, in broadcasts.WindowInput) broadcasts.OutsideWindow {
	t.Helper()
	got, err := testhelper.Module[*broadcasts.Module](env).OutsideWindow(t.Context(), env.DB.Scoped(workspaceID), in)
	require.NoError(t, err)
	return got
}

func TestAnSMSBroadcastAt21InItsRecipientsZoneHoldsThemUntil8There(t *testing.T) {
	env := testhelper.Setup(t)
	narrowLacunaSMS(t, env)

	got := outsideWindow(t, env, fixtures.LacunaID, broadcasts.WindowInput{
		Channel: channels.SMS, At: time.Date(2027, time.January, 15, 21, 0, 0, 0, newYork),
	})

	assert.Equal(t, 4, got.Held, "Ada, Bea, Cyd and Dot; Uma has no Confirmation and Sal is suppressed")
	require.NotNil(t, got.ReachedAt)
	assert.WithinDuration(t, time.Date(2027, time.January, 16, 8, 0, 0, 0, newYork), *got.ReachedAt, 0)
}

func TestAnSMSBroadcastInsideTheWindowHoldsNone(t *testing.T) {
	env := testhelper.Setup(t)
	narrowLacunaSMS(t, env)

	got := outsideWindow(t, env, fixtures.LacunaID, broadcasts.WindowInput{
		Channel: channels.SMS, At: time.Date(2027, time.January, 15, 14, 0, 0, 0, newYork),
	})

	assert.Zero(t, got.Held)
	assert.Nil(t, got.ReachedAt)
}

// Textham's only number that can be texted is Bea's.
func TestTheWindowCountsOnlyTheAudienceOfTheCondition(t *testing.T) {
	env := testhelper.Setup(t)
	narrowLacunaSMS(t, env)
	textham := env.DB.Segment.GetX(t.Context(), fixtures.SegmentLacunaTexthamID)

	got := outsideWindow(t, env, fixtures.LacunaID, broadcasts.WindowInput{
		Condition: &textham.Condition, Channel: channels.SMS, At: time.Date(2027, time.January, 15, 21, 0, 0, 0, newYork),
	})

	assert.Equal(t, 1, got.Held)
}

// Ada lives in Los Angeles: her window opens when it is 8:00 there, three hours after the others'.
func TestTheLastRecipientIsReachedWhenTheLatestWindowOpens(t *testing.T) {
	env := testhelper.Setup(t)
	narrowLacunaSMS(t, env)
	env.DB.Contact.UpdateOneID(fixtures.ContactLacunaAdaID).SetTimeZone("America/Los_Angeles").ExecX(t.Context())

	got := outsideWindow(t, env, fixtures.LacunaID, broadcasts.WindowInput{
		Channel: channels.SMS, At: time.Date(2027, time.January, 15, 21, 0, 0, 0, newYork),
	})

	assert.Equal(t, 4, got.Held)
	require.NotNil(t, got.ReachedAt)
	assert.WithinDuration(t, time.Date(2027, time.January, 16, 8, 0, 0, 0, losAngeles), *got.ReachedAt, 0)
}

// From 8:00 to 20:00, New York and Bangkok, twelve hours apart in January, are never open at once:
// Ada in Bangkok has no local time, so she is Skipped, not held.
func TestARecipientWithNoLocalTimeIsNotHeld(t *testing.T) {
	env := testhelper.Setup(t)
	narrowLacunaSMS(t, env)
	env.DB.Contact.UpdateOneID(fixtures.ContactLacunaAdaID).SetTimeZone("Asia/Bangkok").ExecX(t.Context())

	got := outsideWindow(t, env, fixtures.LacunaID, broadcasts.WindowInput{
		Channel: channels.SMS, At: time.Date(2027, time.January, 15, 21, 0, 0, 0, newYork),
	})

	assert.Equal(t, 3, got.Held)
	require.NotNil(t, got.ReachedAt)
	assert.WithinDuration(t, time.Date(2027, time.January, 16, 8, 0, 0, 0, newYork), *got.ReachedAt, 0)
}

// No law sets hours for email: no window holds it.
func TestAnEmailBroadcastHoldsNoneAtAnyHour(t *testing.T) {
	env := testhelper.Setup(t)

	got := outsideWindow(t, env, fixtures.AcmeID, broadcasts.WindowInput{
		Channel: channels.Email, At: time.Date(2027, time.January, 16, 3, 0, 0, 0, newYork),
	})

	assert.Zero(t, got.Held)
	assert.Nil(t, got.ReachedAt)
}
