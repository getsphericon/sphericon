package frequencycap_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/frequencycap"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/postgres"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Globex caps SMS at three a day, and Germany's numbers at one an hour (fixtures/frequency_caps.yml).
const (
	bostonNumber = "+16175550142"
	berlinNumber = "+49301234567"
)

// claimFor runs an empty claim of an SMS to number under the Frequency cap's locks.
func claimFor(t *testing.T, env *testhelper.TestEnv, number string) func() error {
	t.Helper()
	return func() error {
		return testhelper.Module[*frequencycap.Module](env).Within(t.Context(), env.DB.Scoped(fixtures.GlobexID),
			frequencycap.Claim{Channel: channels.SMS, Destinations: []string{number}},
			func(*ent.DestinationLocked, frequencycap.Standings) error { return nil })
	}
}

func lockOf(number string) int64 {
	return postgres.DestinationKey(frequencycap.LockKind, fixtures.GlobexID, string(channels.SMS), number)
}

// A country's row outranks the Channel's for a number of that country, and a row with no maximum
// removes the cap there: a claim for a German number takes no lock, while one for a number of
// another country still waits for its own under the Channel's cap.
func TestARowOfACountryWithNoMaximumRemovesTheCapForItsNumbers(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	env.DB.FrequencyCap.UpdateOneID(fixtures.FrequencyCapGlobexGermanyID).ClearMaxMessages().ClearPeriodMinutes().ExecX(t.Context())

	berlin := env.RunBesideKey(t, lockOf(berlinNumber), claimFor(t, env, berlinNumber))
	boston := env.QueueBehindKey(t, lockOf(bostonNumber), testhelper.Writer{Name: "boston", Run: claimFor(t, env, bostonNumber)})

	require.NoError(t, berlin)
	require.NoError(t, boston["boston"])
}

// A country's own cap holds its numbers: a claim for a German number waits for its lock.
func TestTheCapOfACountryHoldsItsNumbers(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())

	errs := env.QueueBehindKey(t, lockOf(berlinNumber), testhelper.Writer{Name: "berlin", Run: claimFor(t, env, berlinNumber)})

	require.NoError(t, errs["berlin"])
}
