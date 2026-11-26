package sendingwindow_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/sendingwindow"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Globex's texts may go at any hour, but Germany's numbers only from 9:00 to 17:00 on workdays
// (fixtures/sending_windows.yml). 2027-01-15 is a Friday; Berlin is an hour ahead of UTC then.
const (
	bostonNumber = "+16175550142"
	berlinNumber = "+49301234567"
)

var friday = utc(2027, time.January, 15, 10, 0)

func globexSMS(t *testing.T, env *testhelper.TestEnv) sendingwindow.Rule {
	t.Helper()
	rule, err := sendingwindow.New().Rule(t.Context(), env.DB.Scoped(fixtures.GlobexID), channels.SMS)
	require.NoError(t, err)
	return rule
}

func judge(t *testing.T, rule sendingwindow.Rule, number string, contactZone *string, at time.Time) sendingwindow.Judgement {
	t.Helper()
	j, err := rule.Judge(number, contactZone, at)
	require.NoError(t, err)
	return j
}

func TestACountrysWindowOutranksTheChannelsForItsNumbers(t *testing.T) {
	env := testhelper.Setup(t)
	rule := globexSMS(t, env)
	saturday := utc(2027, time.January, 16, 12, 0)

	german := judge(t, rule, berlinNumber, nil, saturday)
	american := judge(t, rule, bostonNumber, nil, saturday)

	assert.False(t, german.Open)
	assert.WithinDuration(t, utc(2027, time.January, 18, 8, 0), german.Opens, 0, "Monday at 9:00 in Berlin")
	assert.True(t, american.Open, "the Channel's window holds the other numbers")
}

// A Contact's time zone narrows the window: at 11:00 in Berlin it is 5:00 in New York, so a Berlin
// number of a Contact in New York waits for 9:00 there, 15:00 in Berlin.
func TestAContactsTimeZoneKeepsTheWindowClosedWhereItIsNight(t *testing.T) {
	env := testhelper.Setup(t)

	j := judge(t, globexSMS(t, env), berlinNumber, new("America/New_York"), friday)

	assert.False(t, j.Open)
	assert.WithinDuration(t, utc(2027, time.January, 15, 14, 0), j.Opens, 0)
}

func TestAContactsTimeZoneThatIsNoZoneCountsForNothing(t *testing.T) {
	env := testhelper.Setup(t)

	j := judge(t, globexSMS(t, env), berlinNumber, new("Mars/Olympus_Mons"), friday)

	assert.True(t, j.Open)
}

// From 9:00 to 17:00 in Berlin and in Tokyo, no moment is open in both.
func TestANumberWhoseZonesNeverShareAnOpenMomentHasNoLocalTime(t *testing.T) {
	env := testhelper.Setup(t)

	j := judge(t, globexSMS(t, env), berlinNumber, new("Asia/Tokyo"), friday)

	assert.True(t, j.NoLocalTime())
}

// A global service number is in no zone, and a Contact's zone does not stand in for the number's.
func TestANumberInNoZoneHasNoLocalTime(t *testing.T) {
	env := testhelper.Setup(t)

	j := judge(t, globexSMS(t, env), "+80012345678", new("America/New_York"), friday)

	assert.True(t, j.NoLocalTime())
}

// A Boston number is in one zone, and a Contact in New York adds none; a Contact in Los Angeles
// adds hers.
func TestAWindowJudgedByOneZoneSaysSo(t *testing.T) {
	env := testhelper.Setup(t)
	rule := globexSMS(t, env)

	alone := judge(t, rule, bostonNumber, nil, friday)
	sameZone := judge(t, rule, bostonNumber, new("America/New_York"), friday)
	otherZone := judge(t, rule, bostonNumber, new("America/Los_Angeles"), friday)

	assert.True(t, alone.SingleZone())
	assert.True(t, sameZone.SingleZone())
	assert.False(t, otherZone.SingleZone())
}

// No law sets hours for email: no window holds it.
func TestAChannelWithNoWindowIsOpenAtEveryMoment(t *testing.T) {
	env := testhelper.Setup(t)
	rule, err := sendingwindow.New().Rule(t.Context(), env.DB.Scoped(fixtures.GlobexID), channels.Email)
	require.NoError(t, err)

	j := judge(t, rule, "ada@example.com", nil, utc(2027, time.January, 16, 3, 0))

	assert.True(t, j.Open)
	assert.False(t, j.Windowed)
}
