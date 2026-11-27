package eligibility_test

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// matrixDay is when the eligibility matrix's opt-outs, Suppressions and Confirmations were made
// (fixtures/unsubscribes.yml, suppressions.yml, confirmations.yml).
var matrixDay = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// statesOf reads the Channel states of every Contact of a Workspace in one call.
func statesOf(t *testing.T, env *testhelper.TestEnv, ws int64) map[int64]eligibility.ContactStates {
	t.Helper()
	s := env.DB.Scoped(ws)
	contacts, err := s.Contact().Query().Order(ent.Asc(contact.FieldID)).All(t.Context())
	require.NoError(t, err)
	states, err := eligibility.New().ChannelStates(t.Context(), s, contacts)
	require.NoError(t, err)
	return states
}

// Each state of the matrix reads as Send eligibility has it for a Broadcast, with what stands
// behind it: the scopes opted out of and when, the Suppression's reason, the Confirmation's
// provenance, on email and on sms; a Contact without a phone has no sms Destination.
func TestChannelStatesOfTheMatrix(t *testing.T) {
	env := testhelper.Setup(t)

	states := statesOf(t, env, fixtures.TyrellID)

	assert.Equal(t, eligibility.ContactStates{
		Email: eligibility.ChannelState{Destination: fixtures.TyrellMatrixCleanEmail, State: eligibility.StateMayReceive},
		SMS:   eligibility.ChannelState{Destination: fixtures.TyrellMatrixCleanPhone, State: eligibility.StateNeedsConfirmation},
	}, states[fixtures.TyrellMatrixCleanID])
	assert.Equal(t, eligibility.ContactStates{
		Email: eligibility.ChannelState{
			Destination: fixtures.TyrellMatrixSuppressedEmail, State: eligibility.StateSuppressed,
			Suppression: &eligibility.SuppressionFact{Reason: entsuppression.ReasonBounce, At: matrixDay},
		},
	}, states[fixtures.TyrellMatrixSuppressedID])
	assert.Equal(t, eligibility.ContactStates{
		Email: eligibility.ChannelState{
			Destination: fixtures.TyrellMatrixUnsubEverythingEmail, State: eligibility.StateOptedOut,
			OptOuts: []eligibility.OptOut{{Scope: eligibility.SourceEverything, At: matrixDay}},
		},
		SMS: eligibility.ChannelState{
			Destination: fixtures.TyrellMatrixUnsubEverythingPhone, State: eligibility.StateOptedOut,
			OptOuts:      []eligibility.OptOut{{Scope: eligibility.SourceEverything, At: matrixDay}},
			Confirmation: &eligibility.ConfirmationFact{Provenance: confirmation.ProvenanceImported, At: matrixDay},
		},
	}, states[fixtures.TyrellMatrixUnsubEverythingID])
	assert.Equal(t, eligibility.ChannelState{
		Destination: fixtures.TyrellMatrixUnsubBroadcastsEmail, State: eligibility.StateOptedOut,
		OptOuts: []eligibility.OptOut{{Scope: eligibility.SourceBroadcasts, At: matrixDay}},
	}, states[fixtures.TyrellMatrixUnsubBroadcastsID].Email)
	assert.Equal(t, eligibility.ChannelState{
		Destination: fixtures.TyrellMatrixUnsubAutomationEmail, State: eligibility.StateMayReceive,
		OptOuts: []eligibility.OptOut{{Scope: eligibility.AutomationSource(7), At: matrixDay}},
	}, states[fixtures.TyrellMatrixUnsubAutomationID].Email, "an opt-out of one Automation leaves Broadcasts")
	assert.Equal(t, eligibility.ChannelState{
		Destination: fixtures.TyrellMatrixConfirmedSuppressedEmail, State: eligibility.StateSuppressed,
		Suppression:  &eligibility.SuppressionFact{Reason: entsuppression.ReasonBounce, At: matrixDay},
		Confirmation: &eligibility.ConfirmationFact{Provenance: confirmation.ProvenanceDoubleOptIn, At: matrixDay},
	}, states[fixtures.TyrellMatrixConfirmedSuppressedID].Email)
}

// A Destination both opted out of Broadcasts and suppressed reports both: the state is the
// Suppression, which comes first, and the scope stays beside it.
func TestChannelStateOfADestinationOptedOutAndSuppressed(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.TyrellID)
	moved, err := s.Unsubscribe().Update().
		Where(unsubscribe.ChannelEQ(channels.Email), unsubscribe.DestinationEQ(fixtures.TyrellMatrixUnsubBroadcastsEmail)).
		SetDestination(fixtures.TyrellMatrixSuppressedEmail).
		Save(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, moved)

	states := statesOf(t, env, fixtures.TyrellID)

	assert.Equal(t, eligibility.ChannelState{
		Destination: fixtures.TyrellMatrixSuppressedEmail, State: eligibility.StateSuppressed,
		OptOuts:     []eligibility.OptOut{{Scope: eligibility.SourceBroadcasts, At: matrixDay}},
		Suppression: &eligibility.SuppressionFact{Reason: entsuppression.ReasonBounce, At: matrixDay},
	}, states[fixtures.TyrellMatrixSuppressedID].Email)
}

// The time of an opt-out is when the person stopped (ADR 0154), which its consent stamp keeps,
// not when the row was written.
func TestChannelStateTellsWhenTheOptOutOccurred(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.TyrellID)
	stopped := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	_, err := unsubscribes.New().Add(t.Context(), s, unsubscribes.OptOut{
		Channel: channels.Email, Destination: fixtures.TyrellMatrixCleanEmail, Source: eligibility.SourceBroadcasts,
		ContactID: fixtures.TyrellMatrixCleanID, At: consentstamps.At(stopped),
	})
	require.NoError(t, err)

	states := statesOf(t, env, fixtures.TyrellID)

	assert.Equal(t, []eligibility.OptOut{{Scope: eligibility.SourceBroadcasts, At: stopped}},
		states[fixtures.TyrellMatrixCleanID].Email.OptOuts)
}

// On Telegram a Contact is reached at its Bindings: one with two chats that may both receive reads
// as its first Binding, one whose only chat stopped Broadcasts as opted out, one without a Binding
// as no Destination.
func TestChannelStatesOnTelegram(t *testing.T) {
	env := testhelper.Setup(t)

	states := statesOf(t, env, fixtures.LacunaID)

	assert.Equal(t, eligibility.ChannelState{Destination: "77401", State: eligibility.StateMayReceive},
		states[fixtures.ContactLacunaTwoChatsID].Telegram)
	assert.Equal(t, eligibility.ChannelState{
		Destination: "77407", State: eligibility.StateOptedOut,
		OptOuts: []eligibility.OptOut{{Scope: eligibility.SourceBroadcasts, At: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)}},
	}, states[fixtures.ContactLacunaStoppedID].Telegram)
	assert.Equal(t, eligibility.ChannelState{}, states[fixtures.ContactLacunaAdaID].Telegram)
}

// stateOfReason is the Channel state each reason Send eligibility gives a Broadcast means, "" for
// a Destination it admits.
var stateOfReason = map[skipreason.Reason]eligibility.State{
	"":                                eligibility.StateMayReceive,
	skipreason.Suppressed:             eligibility.StateSuppressed,
	skipreason.UnsubscribedEverything: eligibility.StateOptedOut,
	skipreason.UnsubscribedSource:     eligibility.StateOptedOut,
	skipreason.Unconfirmed:            eligibility.StateNeedsConfirmation,
}

// checkedStates returns, for every Channel state with a Destination, the state the read gives
// and the state Send eligibility's point Check gives its Destination for a Broadcast.
func checkedStates(t *testing.T, env *testhelper.TestEnv, ws int64) (read, checked []string) {
	t.Helper()
	states := statesOf(t, env, ws)
	for id := range states {
		for channel, st := range map[channels.Channel]eligibility.ChannelState{
			channels.Email: states[id].Email, channels.SMS: states[id].SMS, channels.Telegram: states[id].Telegram,
		} {
			if st.Destination == "" {
				continue
			}
			d, err := eligibility.New().Check(t.Context(), env.DB.Scoped(ws), eligibility.CheckInput{
				Channel: channel, Destination: st.Destination, Mode: eligibility.Marketing(eligibility.SourceBroadcasts),
			})
			require.NoError(t, err)
			key := string(channel) + " " + st.Destination + " of " + strconv.FormatInt(id, 10)
			read = append(read, key+": "+string(st.State))
			checked = append(checked, key+": "+string(stateOfReason[d.Reason]))
		}
	}
	return read, checked
}

// The read agrees with Send eligibility on every case its tests hold: both confirmed opt-in
// policies, sms with its Confirmation and an opt-out kept under the number's hash, Telegram's
// Bindings, and a lifted Suppression.
func TestChannelStatesAgreeWithSendEligibility(t *testing.T) {
	env := testhelper.Setup(t)
	hashed, err := env.DB.Scoped(fixtures.TyrellID).Unsubscribe().Update().
		Where(unsubscribe.ChannelEQ(channels.SMS), unsubscribe.DestinationEQ(fixtures.TyrellMatrixUnsubEverythingPhone)).
		SetDestination(phone.Hash(fixtures.TyrellMatrixUnsubEverythingPhone)).
		Save(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, hashed)

	for name, ws := range map[string]int64{
		"gate off": fixtures.TyrellID, "gate on": fixtures.LumonID, "bindings": fixtures.LacunaID, "lifted": fixtures.AcmeID,
	} {
		read, checked := checkedStates(t, env, ws)

		assert.NotEmpty(t, read, name)
		assert.ElementsMatch(t, checked, read, name)
	}
	assert.Equal(t, eligibility.StateOptedOut, statesOf(t, env, fixtures.TyrellID)[fixtures.TyrellMatrixUnsubEverythingID].SMS.State,
		"an opt-out kept under the number's hash still holds")
	assert.Equal(t, eligibility.StateMayReceive, statesOf(t, env, fixtures.AcmeID)[fixtures.ContactLiftedID].Email.State,
		"a lifted Suppression no longer holds")
}

// countQueries counts the queries the client runs from now on.
func countQueries(env *testhelper.TestEnv) *atomic.Int64 {
	var n atomic.Int64
	env.DB.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
			n.Add(1)
			return next.Query(ctx, q)
		})
	}))
	return &n
}

// The read is set-based: a page of many Contacts costs the queries one Contact costs.
func TestChannelStatesReadInAFixedNumberOfQueries(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.LacunaID)
	contacts, err := s.Contact().Query().Order(ent.Asc(contact.FieldID)).All(t.Context())
	require.NoError(t, err)
	require.Greater(t, len(contacts), 5)
	n := countQueries(env)

	_, err = eligibility.New().ChannelStates(t.Context(), s, contacts[:1])
	require.NoError(t, err)
	one := n.Swap(0)
	_, err = eligibility.New().ChannelStates(t.Context(), s, contacts)
	require.NoError(t, err)
	many := n.Load()

	assert.Positive(t, one)
	assert.Equal(t, one, many)
}
