package site_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func sendingRules(t *testing.T, c *siteapi.Client, ws siteapi.WorkspaceId) *siteapi.SiteSendingRulesResource {
	t.Helper()
	got, err := c.SiteWorkspacesGetSendingRules(t.Context(), siteapi.SiteWorkspacesGetSendingRulesParams{WorkspaceId: ws})
	require.NoError(t, err)
	return got
}

// replaceSendingRules replaces Acme's sending rules on the revision the Workspace is at.
func replaceSendingRules(t *testing.T, env *testhelper.TestEnv, c *siteapi.Client, in siteapi.SiteReplaceSendingRulesInput) (*siteapi.SiteSendingRulesResource, error) {
	t.Helper()
	in.Revision = siteapi.Revision(env.WorkspaceRevision(t.Context(), t, string(fixtures.AcmePublicID)))
	return c.SiteWorkspacesReplaceSendingRules(t.Context(), &in,
		siteapi.SiteWorkspacesReplaceSendingRulesParams{WorkspaceId: fixtures.AcmePublicID})
}

func capOf(ch siteapi.FrequencyCapChannel, country string, maxMessages, period int32) siteapi.FrequencyCap {
	c := siteapi.FrequencyCap{Channel: ch, Country: siteapi.NewNilCountryCode(siteapi.CountryCode(country)),
		MaxMessages:   siteapi.NewNilFrequencyCapMaxMessages(siteapi.FrequencyCapMaxMessages(maxMessages)),
		PeriodMinutes: siteapi.NewNilFrequencyCapPeriodMinutes(siteapi.FrequencyCapPeriodMinutes(period))}
	c.Country.Null = country == ""
	return c
}

func hoursOf(opens, closes int32, days ...siteapi.Weekday) []siteapi.SendingWindowHours {
	hours := make([]siteapi.SendingWindowHours, 0, len(days))
	for _, d := range days {
		hours = append(hours, siteapi.SendingWindowHours{Day: d, OpensAt: siteapi.MinuteOfDay(opens), ClosesAt: siteapi.MinuteOfDay(closes)})
	}
	return hours
}

var everyDay = []siteapi.Weekday{siteapi.WeekdayMonday, siteapi.WeekdayTuesday, siteapi.WeekdayWednesday,
	siteapi.WeekdayThursday, siteapi.WeekdayFriday, siteapi.WeekdaySaturday, siteapi.WeekdaySunday}

var workdays = everyDay[:5]

func windowOf(country string, hours []siteapi.SendingWindowHours) siteapi.SendingWindow {
	w := siteapi.SendingWindow{Channel: siteapi.SendingWindowChannelSMS, Country: siteapi.NewNilCountryCode(siteapi.CountryCode(country)), Hours: hours}
	w.Country.Null = country == ""
	return w
}

// asInput is the sending rules as a replacement names them: what a client read and sends back.
func asInput(caps []siteapi.FrequencyCap, windows []siteapi.SendingWindow) siteapi.SiteReplaceSendingRulesInput {
	in := siteapi.SiteReplaceSendingRulesInput{
		FrequencyCaps:  make([]siteapi.FrequencyCapInput, 0, len(caps)),
		SendingWindows: make([]siteapi.SendingWindowInput, 0, len(windows)),
	}
	for _, c := range caps {
		in.FrequencyCaps = append(in.FrequencyCaps, siteapi.FrequencyCapInput(c))
	}
	for _, w := range windows {
		hours := make([]siteapi.SendingWindowHoursInput, 0, len(w.Hours))
		for _, h := range w.Hours {
			hours = append(hours, siteapi.SendingWindowHoursInput(h))
		}
		in.SendingWindows = append(in.SendingWindows, siteapi.SendingWindowInput{Channel: w.Channel, Country: w.Country, Hours: hours})
	}
	return in
}

// The defaults: three marketing texts per 24 hours to one number, and texts from 8:00 to 20:00
// every day (ADR 0134, ADR 0137); email and Telegram have neither.
var (
	smsDefaultCap    = capOf(siteapi.FrequencyCapChannelSMS, "", 3, 1440)
	smsDefaultWindow = windowOf("", hoursOf(480, 1200, everyDay...))
)

// acmeWindow is Acme's SMS window in the fixtures: open all day, so that no test that sends a text
// is held by the hour it runs at (fixtures/sending_windows.yml).
var acmeWindow = windowOf("", hoursOf(0, 1440, everyDay...))

func TestAWorkspaceCreatedNowHasTheSMSCapAndWindow(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	created, err := owner.SiteWorkspacesCreate(t.Context(), &siteapi.SiteCreateWorkspaceInput{Name: "Second Brand"})
	require.NoError(t, err)

	got := sendingRules(t, owner, created.ID)

	assert.Equal(t, []siteapi.FrequencyCap{smsDefaultCap}, got.FrequencyCaps)
	assert.Equal(t, []siteapi.SendingWindow{smsDefaultWindow}, got.SendingWindows)
}

// An Owner caps email too, makes SMS stricter in one country and removes the cap in another, and
// narrows one country's window; every member reads the result, and the change is one Audit entry.
func TestAnOwnerReplacesTheSendingRulesAndTheChangeIsAudited(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	before := sendingRules(t, owner, fixtures.AcmePublicID)
	caps := []siteapi.FrequencyCap{
		capOf(siteapi.FrequencyCapChannelEmail, "", 5, 10080),
		smsDefaultCap,
		capOf(siteapi.FrequencyCapChannelSMS, "US", 1, 1440),
		{Channel: siteapi.FrequencyCapChannelSMS, Country: siteapi.NewNilCountryCode("DE"),
			MaxMessages: siteapi.NilFrequencyCapMaxMessages{Null: true}, PeriodMinutes: siteapi.NilFrequencyCapPeriodMinutes{Null: true}},
	}
	windows := []siteapi.SendingWindow{windowOf("US", hoursOf(540, 1080, workdays...)), smsDefaultWindow}

	replaced, err := replaceSendingRules(t, env, owner, asInput(caps, windows))
	require.NoError(t, err)

	wantCaps := []siteapi.FrequencyCap{caps[0], caps[1], caps[3], caps[2]}
	wantWindows := []siteapi.SendingWindow{smsDefaultWindow, windows[0]}
	assert.Equal(t, wantCaps, replaced.FrequencyCaps)
	assert.Equal(t, wantWindows, replaced.SendingWindows)
	assert.Greater(t, replaced.Revision, before.Revision)
	read := sendingRules(t, env.SiteActor(t, fixtures.MemberMaryEmail), fixtures.AcmePublicID)
	assert.Equal(t, wantCaps, read.FrequencyCaps)
	assert.Equal(t, wantWindows, read.SendingWindows)
	published := testhelper.Queued[*events.AuditEntry](t, env, events.NameAuditEntry)
	require.Len(t, published, 1)
	entry := published[0]
	assert.Equal(t, events.ActionWorkspaceSendingRulesChange, entry.Action)
	diff, err := json.Marshal(entry.Diff)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"frequency_caps": {
			"from": [{"channel": "sms", "country": null, "maxMessages": 3, "periodMinutes": 1440}],
			"to": [
				{"channel": "email", "country": null, "maxMessages": 5, "periodMinutes": 10080},
				{"channel": "sms", "country": null, "maxMessages": 3, "periodMinutes": 1440},
				{"channel": "sms", "country": "DE", "maxMessages": null, "periodMinutes": null},
				{"channel": "sms", "country": "US", "maxMessages": 1, "periodMinutes": 1440}
			]
		},
		"sending_windows": {
			"from": [{"channel": "sms", "country": null, "hours": [
				{"day": "monday", "opensAt": 0, "closesAt": 1440}, {"day": "tuesday", "opensAt": 0, "closesAt": 1440},
				{"day": "wednesday", "opensAt": 0, "closesAt": 1440}, {"day": "thursday", "opensAt": 0, "closesAt": 1440},
				{"day": "friday", "opensAt": 0, "closesAt": 1440}, {"day": "saturday", "opensAt": 0, "closesAt": 1440},
				{"day": "sunday", "opensAt": 0, "closesAt": 1440}]}],
			"to": [
				{"channel": "sms", "country": null, "hours": [
					{"day": "monday", "opensAt": 480, "closesAt": 1200}, {"day": "tuesday", "opensAt": 480, "closesAt": 1200},
					{"day": "wednesday", "opensAt": 480, "closesAt": 1200}, {"day": "thursday", "opensAt": 480, "closesAt": 1200},
					{"day": "friday", "opensAt": 480, "closesAt": 1200}, {"day": "saturday", "opensAt": 480, "closesAt": 1200},
					{"day": "sunday", "opensAt": 480, "closesAt": 1200}]},
				{"channel": "sms", "country": "US", "hours": [
					{"day": "monday", "opensAt": 540, "closesAt": 1080}, {"day": "tuesday", "opensAt": 540, "closesAt": 1080},
					{"day": "wednesday", "opensAt": 540, "closesAt": 1080}, {"day": "thursday", "opensAt": 540, "closesAt": 1080},
					{"day": "friday", "opensAt": 540, "closesAt": 1080}]}
			]
		}
	}`, string(diff))
	recorded := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionWorkspaceSendingRulesChange)
	require.Len(t, recorded, 1)
	assert.Equal(t, fixtures.OwnerJohnName, recorded[0].Actor.Name.Value)
}

// Removing every cap leaves the Workspace with none, and the window's change alone is what the
// Audit entry names when the caps stay.
func TestAnAuditEntryNamesOnlyTheListThatChanged(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := replaceSendingRules(t, env, owner, asInput([]siteapi.FrequencyCap{smsDefaultCap},
		[]siteapi.SendingWindow{windowOf("", hoursOf(480, 1260, everyDay...))}))
	require.NoError(t, err)

	published := testhelper.Queued[*events.AuditEntry](t, env, events.NameAuditEntry)
	require.Len(t, published, 1)
	assert.Contains(t, published[0].Diff, "sending_windows")
	assert.NotContains(t, published[0].Diff, "frequency_caps")
}

func TestAReplacementThatChangesNothingIsNotAudited(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	before := sendingRules(t, owner, fixtures.AcmePublicID)

	after, err := replaceSendingRules(t, env, owner, asInput(before.FrequencyCaps, before.SendingWindows))
	require.NoError(t, err)

	assert.Equal(t, before, after)
	audited := env.QueuedEvents(t, events.NameAuditEntry)
	assert.Empty(t, audited)
}

// A form read before another change is refused rather than saved over it (ADR 0119).
func TestAReplacementOnAStaleRevisionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	in := asInput([]siteapi.FrequencyCap{capOf(siteapi.FrequencyCapChannelSMS, "", 2, 1440)}, []siteapi.SendingWindow{smsDefaultWindow})
	// A revision other than the Workspace's: the one a form read before a change since.
	in.Revision = siteapi.Revision(env.WorkspaceRevision(t.Context(), t, string(fixtures.AcmePublicID)) + 1)

	_, err := owner.SiteWorkspacesReplaceSendingRules(t.Context(), &in,
		siteapi.SiteWorkspacesReplaceSendingRulesParams{WorkspaceId: fixtures.AcmePublicID})

	testhelper.SiteRefusal(t, err, http.StatusConflict)
	stored := sendingRules(t, owner, fixtures.AcmePublicID)
	assert.Equal(t, []siteapi.FrequencyCap{smsDefaultCap}, stored.FrequencyCaps)
}

// A window that never opens in a week would hold its messages for ever (ADR 0134), a country's as
// much as the Channel's.
func TestAWindowThatNeverOpensIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	caps := []siteapi.FrequencyCap{smsDefaultCap}

	_, channelErr := replaceSendingRules(t, env, owner, asInput(caps, []siteapi.SendingWindow{windowOf("", nil)}))
	_, countryErr := replaceSendingRules(t, env, owner, asInput(caps, []siteapi.SendingWindow{smsDefaultWindow, windowOf("FR", nil)}))

	channel := testhelper.SiteRefusal(t, channelErr, http.StatusUnprocessableEntity)
	assert.Contains(t, channel.Errors.Value["sendingWindows.0"][0], "never opens")
	country := testhelper.SiteRefusal(t, countryErr, http.StatusUnprocessableEntity)
	assert.Contains(t, country.Errors.Value["sendingWindows.1"][0], "never opens")
	stored := sendingRules(t, owner, fixtures.AcmePublicID)
	assert.Equal(t, []siteapi.SendingWindow{acmeWindow}, stored.SendingWindows)
	audited := env.QueuedEvents(t, events.NameAuditEntry)
	assert.Empty(t, audited)
}

func TestACapPeriodOutsideOneMinuteToSevenDaysIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	windows := []siteapi.SendingWindow{smsDefaultWindow}

	_, zero := replaceSendingRules(t, env, owner, asInput([]siteapi.FrequencyCap{capOf(siteapi.FrequencyCapChannelSMS, "", 3, 0)}, windows))
	_, overAWeek := replaceSendingRules(t, env, owner, asInput([]siteapi.FrequencyCap{capOf(siteapi.FrequencyCapChannelSMS, "", 3, 10081)}, windows))

	testhelper.SiteRefusal(t, zero, http.StatusBadRequest)
	testhelper.SiteRefusal(t, overAWeek, http.StatusBadRequest)
	stored := sendingRules(t, owner, fixtures.AcmePublicID)
	assert.Equal(t, []siteapi.FrequencyCap{smsDefaultCap}, stored.FrequencyCaps)
	audited := env.QueuedEvents(t, events.NameAuditEntry)
	assert.Empty(t, audited)
}

// The rest of what a replacement may not hold: a country off a phone Channel, a maximum without a
// period, two caps for one Channel and country, a day listed twice, hours that do not close after
// they open.
func TestARuleTheChannelOrItsShapeCannotHoldIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	windows := []siteapi.SendingWindow{smsDefaultWindow}
	noPeriod := capOf(siteapi.FrequencyCapChannelSMS, "", 3, 1440)
	noPeriod.PeriodMinutes.Null = true
	cases := []struct {
		name    string
		in      siteapi.SiteReplaceSendingRulesInput
		field   string
		message string
	}{
		{"a country on email", asInput([]siteapi.FrequencyCap{capOf(siteapi.FrequencyCapChannelEmail, "US", 3, 1440)}, windows),
			"frequencyCaps.0", "only on a phone channel"},
		{"a maximum without a period", asInput([]siteapi.FrequencyCap{noPeriod}, windows), "frequencyCaps.0", "both a maximum and a period"},
		{"two caps for SMS", asInput([]siteapi.FrequencyCap{smsDefaultCap, capOf(siteapi.FrequencyCapChannelSMS, "", 2, 60)}, windows),
			"frequencyCaps.1", "already have a rule"},
		{"two windows for one country", asInput(nil, []siteapi.SendingWindow{windowOf("US", hoursOf(480, 1200, everyDay...)), windowOf("US", hoursOf(540, 1200, everyDay...))}),
			"sendingWindows.1", "already have a rule"},
		{"a day listed twice", asInput(nil, []siteapi.SendingWindow{windowOf("", hoursOf(480, 1200, siteapi.WeekdayMonday, siteapi.WeekdayMonday))}),
			"sendingWindows.0.hours.1", "listed twice"},
		{"a window that closes as it opens", asInput(nil, []siteapi.SendingWindow{windowOf("", hoursOf(600, 600, siteapi.WeekdayMonday))}),
			"sendingWindows.0.hours.0", "closes after it opens"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := replaceSendingRules(t, env, owner, c.in)

			refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
			assert.Contains(t, refused.Errors.Value[c.field][0], c.message)
		})
	}
	audited := env.QueuedEvents(t, events.NameAuditEntry)
	assert.Empty(t, audited)
}

func TestAMemberCannotChangeTheSendingRules(t *testing.T) {
	env := testhelper.Setup(t)
	in := asInput(nil, []siteapi.SendingWindow{smsDefaultWindow})

	_, member := replaceSendingRules(t, env, env.SiteActor(t, fixtures.MemberMaryEmail), in)
	_, outsider := replaceSendingRules(t, env, env.SiteActor(t, fixtures.OutsiderOscarEmail), in)

	testhelper.SiteRefusal(t, member, http.StatusForbidden)
	testhelper.SiteRefusal(t, outsider, http.StatusNotFound)
	stored := sendingRules(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)
	assert.Equal(t, []siteapi.FrequencyCap{smsDefaultCap}, stored.FrequencyCaps)
	audited := env.QueuedEvents(t, events.NameAuditEntry)
	assert.Empty(t, audited)
}
