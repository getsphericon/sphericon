package condition_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Contacts of the engagement scenarios. Massive: Ann opened no email, Bob one by hand (and two
// by software), Cara two by hand; each has an email address. Duff: Tara is in two Telegram chats
// and Otis in one, and neither has an email address. Krusty: Kim and Cora have a phone each.
const (
	ann, bob, cara = fixtures.MassiveAnnID, fixtures.MassiveBobID, fixtures.MassiveCaraID
	tara, otis     = fixtures.ContactDuffTwoChatsID, fixtures.ContactDuffOneChatID
	kim, cora      = fixtures.ContactKrustyTexterID, fixtures.ContactKrustyConfirmedID
)

// verdicts is what a Condition makes of some Contacts of a Workspace: the ones it matches, and the
// ones it leaves out because an engagement count is unknown for them (ADR 0123).
type verdicts struct {
	matched, leftOut []int64
}

// tb is what judge needs of a test: a *testing.T or a *rapid.T.
type tb interface {
	Helper()
	require.TestingT
}

// judge compiles g for the Workspace ws and reads which of the Contacts among it matches and which
// it leaves out for unknown engagement.
func judge(t tb, env *testhelper.TestEnv, ws int64, g conditiontree.Segment, among ...int64) verdicts {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(ws)
	m := condition.New(time.Now)
	matches, err := m.Compile(ctx, s, g)
	require.NoError(t, err)
	unknown, err := m.LeftOut(ctx, s, g)
	require.NoError(t, err)
	matched, err := s.Contact().Query().Where(contact.IDIn(among...), matches).Order(contact.ByID()).IDs(ctx)
	require.NoError(t, err)
	leftOut, err := s.Contact().Query().Where(contact.IDIn(among...), unknown).Order(contact.ByID()).IDs(ctx)
	require.NoError(t, err)
	return verdicts{matched: lo.Ternary(len(matched) == 0, nil, matched), leftOut: lo.Ternary(len(leftOut) == 0, nil, leftOut)}
}

// record records a member's observation of a Destination's Engagement tracking consent.
func record(t *testing.T, env *testhelper.TestEnv, ws int64, channel channels.Channel, destination string, state engagementtracking.State) {
	t.Helper()
	_, err := testhelper.Module[*engagementtracking.Module](env).Record(t.Context(), env.DB.Scoped(ws), engagementtracking.RecordInput{
		Channel: channel, Destination: destination, State: state, Provenance: engagementtracking.Manual,
	})
	require.NoError(t, err)
}

// dontTrack turns the Workspace's Engagement tracking default to dont_track.
func dontTrack(t *testing.T, env *testhelper.TestEnv, ws int64) {
	t.Helper()
	s := env.DB.Scoped(ws)
	w, err := s.Workspace(t.Context())
	require.NoError(t, err)
	_, err = testhelper.Module[*workspaces.Module](env).SetEngagementTrackingDefault(t.Context(), s, w.Revision, workspace.EngagementTrackingDefaultDontTrack)
	require.NoError(t, err)
}

// An email engagement count is unknown, in either direction and under a negation, for a Contact
// whose address refused tracking: Ann and Bob refused, Cara is tracked and keeps her answer.
func TestAnEmailEngagementCountIsUnknownForARefusedAddress(t *testing.T) {
	env := testhelper.Setup(t)
	record(t, env, fixtures.MassiveID, channels.Email, fixtures.MassiveAnnEmail, engagementtracking.Denied)
	record(t, env, fixtures.MassiveID, channels.Email, fixtures.MassiveBobEmail, engagementtracking.Denied)
	opened := func(op string, times int) leaf { return eventLeaf(systemevent.EmailOpened, op, times, 90) }

	atMostTwo := judge(t, env, fixtures.MassiveID, segmentOf(opened("atMost", 2)), ann, bob, cara)
	atMostNone := judge(t, env, fixtures.MassiveID, segmentOf(opened("atMost", 0)), ann, bob, cara)
	notAtMostNone := judge(t, env, fixtures.MassiveID, segmentOf(negated(opened("atMost", 0))), ann, bob, cara)
	atLeastOne := judge(t, env, fixtures.MassiveID, segmentOf(opened("atLeast", 1)), ann, bob, cara)
	notAtLeastOne := judge(t, env, fixtures.MassiveID, segmentOf(negated(opened("atLeast", 1))), ann, bob, cara)
	clicked := judge(t, env, fixtures.MassiveID, segmentOf(eventLeaf(systemevent.EmailClicked, "atMost", 0, 0)), ann, bob, cara)

	assert.Equal(t, verdicts{matched: []int64{cara}, leftOut: []int64{ann, bob}}, atMostTwo, "everyone opened at most twice, but only Cara is known to")
	assert.Equal(t, verdicts{leftOut: []int64{ann, bob}}, atMostNone, "Ann opened nothing seen, which is not none")
	assert.Equal(t, verdicts{matched: []int64{cara}, leftOut: []int64{ann, bob}}, notAtMostNone)
	assert.Equal(t, verdicts{matched: []int64{cara}, leftOut: []int64{ann, bob}}, atLeastOne, "Bob's open is not seen either")
	assert.Equal(t, verdicts{leftOut: []int64{ann, bob}}, notAtLeastOne, "a negation does not put them back")
	assert.Equal(t, verdicts{matched: []int64{cara}, leftOut: []int64{ann, bob}}, clicked, "a click is engagement too, and Cara clicked nothing")
}

// Under a dont_track default an address is tracked only where it granted: Cara did, Ann and Bob
// recorded nothing.
func TestAnEmailEngagementCountIsUnknownUnderDontTrackWithoutAGrant(t *testing.T) {
	env := testhelper.Setup(t)
	before := judge(t, env, fixtures.MassiveID, segmentOf(eventLeaf(systemevent.EmailOpened, "atMost", 1, 0)), ann, bob, cara)
	dontTrack(t, env, fixtures.MassiveID)
	record(t, env, fixtures.MassiveID, channels.Email, fixtures.MassiveCaraEmail, engagementtracking.Granted)

	atMostOne := judge(t, env, fixtures.MassiveID, segmentOf(eventLeaf(systemevent.EmailOpened, "atMost", 1, 0)), ann, bob, cara)
	notAtMostOne := judge(t, env, fixtures.MassiveID, segmentOf(negated(eventLeaf(systemevent.EmailOpened, "atMost", 1, 0))), ann, bob, cara)

	assert.Equal(t, verdicts{matched: []int64{ann, bob}}, before, "the default tracks until it is turned")
	assert.Equal(t, verdicts{leftOut: []int64{ann, bob}}, atMostOne)
	assert.Equal(t, verdicts{matched: []int64{cara}, leftOut: []int64{ann, bob}}, notAtMostOne, "Cara's grant keeps her known")
}

// A Contact with no address keeps the two-valued answer of an email count, whatever the default:
// nothing was sent there to be tracked. Tara and Otis have no email address.
func TestAnEmailEngagementCountStaysKnownForAContactWithNoAddress(t *testing.T) {
	env := testhelper.Setup(t)
	dontTrack(t, env, fixtures.DuffID)

	none := judge(t, env, fixtures.DuffID, segmentOf(eventLeaf(systemevent.EmailOpened, "atMost", 0, 90)), tara, otis)
	some := judge(t, env, fixtures.DuffID, segmentOf(negated(eventLeaf(systemevent.EmailOpened, "atMost", 0, 90))), tara, otis)

	assert.Equal(t, verdicts{matched: []int64{tara, otis}}, none)
	assert.Equal(t, verdicts{}, some)
}

// A Telegram click count is unknown when any of the Contact's chats is not tracked: Tara refused in
// one of her two chats, Otis in none of his.
func TestATelegramEngagementCountIsUnknownWhenAnyChatIsNotTracked(t *testing.T) {
	env := testhelper.Setup(t)
	record(t, env, fixtures.DuffID, channels.Telegram, "77001", engagementtracking.Denied)

	none := judge(t, env, fixtures.DuffID, segmentOf(eventLeaf(systemevent.TelegramClicked, "atMost", 0, 0)), tara, otis)
	some := judge(t, env, fixtures.DuffID, segmentOf(negated(eventLeaf(systemevent.TelegramClicked, "atMost", 0, 0))), tara, otis)

	assert.Equal(t, verdicts{matched: []int64{otis}, leftOut: []int64{tara}}, none)
	assert.Equal(t, verdicts{leftOut: []int64{tara}}, some)
}

// An SMS click count is unknown for a Contact whose phone refused tracking: Kim refused, Cora did
// not.
func TestAnSMSEngagementCountIsUnknownForARefusedPhone(t *testing.T) {
	env := testhelper.Setup(t)
	record(t, env, fixtures.KrustyID, channels.SMS, fixtures.ContactKrustyTexterPhone, engagementtracking.Denied)

	none := judge(t, env, fixtures.KrustyID, segmentOf(eventLeaf(systemevent.SmsClicked, "atMost", 0, 0)), kim, cora)
	some := judge(t, env, fixtures.KrustyID, segmentOf(negated(eventLeaf(systemevent.SmsClicked, "atMost", 0, 0))), kim, cora)
	email := judge(t, env, fixtures.KrustyID, segmentOf(eventLeaf(systemevent.EmailOpened, "atMost", 0, 0)), kim, cora)

	assert.Equal(t, verdicts{matched: []int64{cora}, leftOut: []int64{kim}}, none)
	assert.Equal(t, verdicts{leftOut: []int64{kim}}, some)
	assert.Equal(t, verdicts{matched: []int64{kim, cora}}, email, "the phone's refusal is not the address's")
}

// Unknown passes through a group by Kleene's rules: a test that holds decides an OR, one that fails
// decides an AND, and only engagement makes a Contact left out. Ann refused tracking.
func TestAnUnknownEngagementCountComposesByKleenesRules(t *testing.T) {
	env := testhelper.Setup(t)
	record(t, env, fixtures.MassiveID, channels.Email, fixtures.MassiveAnnEmail, engagementtracking.Denied)
	opened := eventLeaf(systemevent.EmailOpened, "atLeast", 1, 0)
	isAnn, isBob := attribute("first_name", text("eq", "Ann")), attribute("first_name", text("eq", "Bob"))

	annOrOpened := judge(t, env, fixtures.MassiveID, segmentOf(groupOf(conditiontree.Or, false, isAnn, opened)), ann, bob, cara)
	bobOrOpened := judge(t, env, fixtures.MassiveID, segmentOf(groupOf(conditiontree.Or, false, isBob, opened)), ann, bob, cara)
	annAndOpened := judge(t, env, fixtures.MassiveID, segmentOf(isAnn, opened), ann, bob, cara)
	bobAndOpened := judge(t, env, fixtures.MassiveID, segmentOf(isBob, opened), ann, bob, cara)

	assert.Equal(t, verdicts{matched: []int64{ann, bob, cara}}, annOrOpened, "Ann's name holds, so her unknown opens do not matter")
	assert.Equal(t, verdicts{matched: []int64{bob, cara}, leftOut: []int64{ann}}, bobOrOpened)
	assert.Equal(t, verdicts{leftOut: []int64{ann}}, annAndOpened)
	assert.Equal(t, verdicts{matched: []int64{bob}}, bobAndOpened, "Ann is not Bob, so her unknown opens do not matter")
}

// A test of a field that reads no value is not engagement: Tara and Otis have no address, so
// `email neq` is no answer for them, and the Condition leaves nobody out for engagement although
// it matches nobody.
func TestATestOfAnEmptyFieldLeavesNobodyOutForEngagement(t *testing.T) {
	env := testhelper.Setup(t)
	notThatAddress := attribute("email", text("neq", "x@example.com"))
	clicked := eventLeaf(systemevent.TelegramClicked, "atMost", 0, 0)

	got := judge(t, env, fixtures.DuffID, segmentOf(notThatAddress, clicked), tara, otis)

	assert.Equal(t, verdicts{}, got)
}

// Whatever the tree, a Contact is matched exactly when Kleene's logic over its leaves is true, and
// left out exactly when it is unknown. Ann and Bob refused tracking, Cara is tracked.
func TestUnknownEngagementFollowsKleenesLogicAtAnyDepth(t *testing.T) {
	env := testhelper.Setup(t)
	record(t, env, fixtures.MassiveID, channels.Email, fixtures.MassiveAnnEmail, engagementtracking.Denied)
	record(t, env, fixtures.MassiveID, channels.Email, fixtures.MassiveBobEmail, engagementtracking.Denied)
	everyone := []int64{ann, bob, cara}

	rapid.Check(t, func(rt *rapid.T) {
		g := kleeneTree(rt, 0)

		got := judge(rt, env, fixtures.MassiveID, conditiontree.Segment{Group: g.group()}, everyone...)

		assert.Equal(rt, g.expected(everyone), got)
	})
}
