package consentstamps_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An observation changes a consent state only when it occurred later than the one that set it
// (ADR 0154): within one bot by its update_id, otherwise by time, and of the same moment the
// refusal wins.
func TestAnObservationIsAdmittedOnlyWhenItOccurredLater(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	const botA, botB = 1, 2
	cases := []struct {
		name     string
		standing consentstamps.Occurrence
		next     consentstamps.Occurrence
		refusal  bool
		admitted bool
	}{
		{"a later grant", consentstamps.At(t0), consentstamps.At(t0.Add(time.Second)), false, true},
		{"an earlier grant", consentstamps.At(t0), consentstamps.At(t0.Add(-time.Second)), false, false},
		{"a grant of the same moment", consentstamps.At(t0), consentstamps.At(t0), false, false},
		{"a refusal of the same moment", consentstamps.At(t0), consentstamps.At(t0), true, true},
		{"a refusal of the same microsecond", consentstamps.At(t0), consentstamps.At(t0.Add(time.Nanosecond)), true, true},
		{"a later update of the bot dated earlier", consentstamps.Occurrence{At: t0, IntegrationID: botA, Sequence: 100},
			consentstamps.Occurrence{At: t0.Add(-time.Minute), IntegrationID: botA, Sequence: 101}, false, true},
		{"an earlier update of the bot dated later", consentstamps.Occurrence{At: t0, IntegrationID: botA, Sequence: 101},
			consentstamps.Occurrence{At: t0.Add(time.Minute), IntegrationID: botA, Sequence: 100}, true, false},
		{"another bot's update is ordered by time", consentstamps.Occurrence{At: t0, IntegrationID: botA, Sequence: 101},
			consentstamps.Occurrence{At: t0.Add(time.Second), IntegrationID: botB, Sequence: 7}, false, true},
		{"an unnumbered observation after a numbered one", consentstamps.Occurrence{At: t0, IntegrationID: botA, Sequence: 101},
			consentstamps.At(t0.Add(-time.Second)), true, false},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			s := env.DB.Scoped(fixtures.AcmeID)
			k := consentstamps.Key{Channel: channels.Telegram, Destination: strconv.Itoa(9100 + i), Source: "everything"}
			first, err := consentstamps.New().Admit(t.Context(), s, consentstamps.Observation{Key: k, Fact: consentstamps.OptOut, Occurred: c.standing, Refusal: true})
			require.NoError(t, err)
			require.True(t, first, "nothing has set the state yet")

			admitted, err := consentstamps.New().Admit(t.Context(), s, consentstamps.Observation{Key: k, Fact: consentstamps.OptOut, Occurred: c.next, Refusal: c.refusal})

			require.NoError(t, err)
			assert.Equal(t, c.admitted, admitted)
		})
	}
}

func TestAnObservationWithoutItsTimeIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	k := consentstamps.Key{Channel: channels.Telegram, Destination: "9199", Source: "everything"}

	_, err := consentstamps.New().Admit(t.Context(), env.DB.Scoped(fixtures.AcmeID), consentstamps.Observation{Key: k, Fact: consentstamps.OptOut, Refusal: true})

	require.ErrorIs(t, err, consentstamps.ErrNoOccurrence)
}
