package condition_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Condition field per Channel reads the Contact's Engagement tracking consent as recorded (ADR
// 0120): granted, denied or not set, live.

// tracking is an engagementTracking leaf on channel.
func tracking(channel string, test leaf) leaf {
	return leaf{"kind": "engagementTracking", "channel": channel, "test": test}
}

// consentConditions compiles Segments of one leaf each in a Workspace and lists who they match.
type consentConditions struct {
	t   *testing.T
	env *testhelper.TestEnv
	ws  int64
}

// matching is the ids of the Workspace's Contacts the Segment of l matches.
func (c consentConditions) matching(l leaf) []int64 {
	c.t.Helper()
	ctx := context.Background()
	s := c.env.DB.Scoped(c.ws)
	p, err := condition.New(time.Now).Compile(ctx, s, segmentOf(l))
	require.NoError(c.t, err)
	ids, err := s.Contact().Query().Where(p).IDs(ctx)
	require.NoError(c.t, err)
	return ids
}

// everyone is the ids of every Contact of the Workspace.
func (c consentConditions) everyone() []int64 {
	c.t.Helper()
	ids, err := c.env.DB.Scoped(c.ws).Contact().Query().IDs(context.Background())
	require.NoError(c.t, err)
	return ids
}

// record records an observation of destination on channel through the consent module.
func (c consentConditions) record(channel channels.Channel, destination string, state engagementtracking.State) {
	c.t.Helper()
	_, err := testhelper.Module[*engagementtracking.Module](c.env).Record(context.Background(), c.env.DB.Scoped(c.ws), engagementtracking.RecordInput{
		Channel: channel, Destination: destination, State: state, Provenance: engagementtracking.Imported,
	})
	require.NoError(c.t, err)
}

// Email reads the consent of the Contact's address: a refused one is denied, a granted one granted,
// and one with nothing recorded, like a Contact with no email, is not set. granted, denied and not
// set split every Contact, and neq is the complement of eq.
func TestAnEmailTrackingConsentConditionReadsTheContactsAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := consentConditions{t: t, env: env, ws: fixtures.AcmeID}
	c.record(channels.Email, fixtures.ContactAliceEmail, engagementtracking.Denied)
	c.record(channels.Email, fixtures.ContactBobEmail, engagementtracking.Granted)

	denied := c.matching(tracking("email", text("eq", "denied")))
	granted := c.matching(tracking("email", text("eq", "granted")))
	notSet := c.matching(tracking("email", presence("notSet")))
	set := c.matching(tracking("email", presence("set")))
	notDenied := c.matching(tracking("email", text("neq", "denied")))
	everyone := c.everyone()

	assert.Equal(t, []int64{fixtures.ContactAliceID}, denied)
	assert.Equal(t, []int64{fixtures.ContactBobID}, granted)
	assert.Contains(t, notSet, int64(fixtures.ContactLiamID), "nothing recorded")
	assert.Contains(t, notSet, int64(fixtures.ContactJobsNoEmailID), "no email")
	assert.ElementsMatch(t, []int64{fixtures.ContactAliceID, fixtures.ContactBobID}, set)
	assert.ElementsMatch(t, everyone, append(append(append([]int64{}, denied...), granted...), notSet...), "the three split every Contact")
	assert.ElementsMatch(t, lo.Without(everyone, fixtures.ContactAliceID), notDenied)
}

// The consent is read as recorded: a Workspace whose default is not to track reads the same, and a
// Contact whose email is not tracked by the default alone is not set.
func TestATrackingConsentConditionIgnoresTheWorkspacesDefault(t *testing.T) {
	env := testhelper.Setup(t)
	c := consentConditions{t: t, env: env, ws: fixtures.AcmeID}
	c.record(channels.Email, fixtures.ContactAliceEmail, engagementtracking.Denied)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ws, err := acme.Workspace(t.Context())
	require.NoError(t, err)
	_, err = testhelper.Module[*workspaces.Module](env).SetEngagementTrackingDefault(t.Context(), acme, ws.Revision, engagementtracking.DontTrack)
	require.NoError(t, err)

	denied := c.matching(tracking("email", text("eq", "denied")))
	notSet := c.matching(tracking("email", presence("notSet")))

	assert.Equal(t, []int64{fixtures.ContactAliceID}, denied)
	assert.Contains(t, notSet, int64(fixtures.ContactBobID))
}

// SMS reads the consent of the Contact's phone, as it was typed in any form of the number.
func TestAnSMSTrackingConsentConditionReadsTheContactsPhone(t *testing.T) {
	env := testhelper.Setup(t)
	c := consentConditions{t: t, env: env, ws: fixtures.AcmeID}
	c.record(channels.SMS, "+1 415 555 0810", engagementtracking.Denied)

	denied := c.matching(tracking("sms", text("eq", "denied")))
	emailDenied := c.matching(tracking("email", text("eq", "denied")))
	notSet := c.matching(tracking("sms", presence("notSet")))

	assert.Equal(t, []int64{fixtures.ContactTexterID}, denied)
	assert.Empty(t, emailDenied, "each Destination keeps its own choice")
	assert.ElementsMatch(t, lo.Without(c.everyone(), fixtures.ContactTexterID), notSet)
}

// Telegram reads the Contact's own chats: denied when any is denied, granted when every one is
// granted, and not set while one has nothing recorded or the Contact has none.
func TestATelegramTrackingConsentConditionReadsEveryChatOfTheContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := consentConditions{t: t, env: env, ws: fixtures.DuffID}
	const two, one = fixtures.ContactDuffTwoChatsID, fixtures.ContactDuffOneChatID
	// readings is who reads as refused, as granted and as not set.
	type readings struct{ refused, agreed, unset []int64 }
	read := func() readings {
		t.Helper()
		return readings{
			refused: c.matching(tracking("telegram", text("eq", "denied"))),
			agreed:  c.matching(tracking("telegram", text("eq", "granted"))),
			unset:   c.matching(tracking("telegram", presence("notSet"))),
		}
	}
	everyone := c.everyone()

	nothing := read()
	c.record(channels.Telegram, "77001", engagementtracking.Granted)
	oneOfTwo := read()
	c.record(channels.Telegram, "77002", engagementtracking.Granted)
	both := read()
	c.record(channels.Telegram, "77002", engagementtracking.Denied)
	oneDenied := read()

	assert.Empty(t, nothing.refused)
	assert.Empty(t, nothing.agreed)
	assert.ElementsMatch(t, everyone, nothing.unset)
	assert.Empty(t, oneOfTwo.agreed, "one chat of two has nothing recorded")
	assert.Contains(t, oneOfTwo.unset, int64(two))
	assert.Equal(t, []int64{two}, both.agreed)
	assert.Equal(t, []int64{two}, oneDenied.refused, "one chat denied of two")
	assert.Empty(t, oneDenied.agreed)
	assert.ElementsMatch(t, lo.Without(everyone, two), oneDenied.unset)
	assert.Contains(t, oneDenied.unset, int64(one), "a Contact whose chat has nothing recorded")
}

// A chat of a Contact's Visitor is not the Contact's (ADR 0171): its refusal leaves the Contact's
// consent not set.
func TestATelegramTrackingConsentConditionLeavesAVisitorsChatOut(t *testing.T) {
	env := testhelper.Setup(t)
	c := consentConditions{t: t, env: env, ws: fixtures.AcmeID}
	c.record(channels.Telegram, fixtures.BindingEmmaPhoneDestination, engagementtracking.Denied)
	c.record(channels.Telegram, fixtures.BindingLiamDestination, engagementtracking.Denied)

	denied := c.matching(tracking("telegram", text("eq", "denied")))

	assert.Equal(t, []int64{fixtures.ContactLiamID}, denied)
}

// What a consent cannot be compared with is refused: a text operator other than eq and neq, a value
// that is no state. (A Channel that takes no consent, or a test of another kind, the contract's
// schema refuses already.)
func TestATrackingConsentConditionRefusesWhatItCannotRead(t *testing.T) {
	env := testhelper.Setup(t)
	refusals := []string{
		`{"kind":"group","combinator":"and","entries":[{"kind":"engagementTracking","channel":"email","test":{"kind":"text","operator":"contains","value":"denied"}}]}`,
		`{"kind":"group","combinator":"and","entries":[{"kind":"engagementTracking","channel":"email","test":{"kind":"text","operator":"eq","value":"default"}}]}`,
	}

	for _, def := range refusals {
		require.Error(t, validate(t, env, def), def)
	}
	require.NoError(t, validate(t, env, `{"kind":"group","combinator":"and","entries":[{"kind":"engagementTracking","channel":"sms","test":{"kind":"text","operator":"neq","value":"granted"}}]}`))
}
