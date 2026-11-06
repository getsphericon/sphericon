package condition_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/confirmations"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/optout"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// validate compiles a condition for the Acme Workspace.
func validate(t *testing.T, env *testhelper.TestEnv, def string) error {
	t.Helper()
	_, err := condition.ContactPredicate(context.Background(), env.DB.Scoped(fixtures.AcmeID), def)
	return err
}

// emailsIn returns, in email order, the emails of the Workspace's Contacts matching g.
func emailsIn(t *testing.T, env *testhelper.TestEnv, ws int64, g condition.Group) []string {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(ws)
	p, err := condition.Compile(ctx, s, g)
	require.NoError(t, err)
	emails, err := s.Contact().Query().Where(p).Order(contact.ByEmail()).Select(contact.FieldEmail).Strings(ctx)
	require.NoError(t, err)
	return emails
}

func rule(field, op, value string) condition.Leaf {
	return condition.Leaf{Kind: "rule", Field: field, Operator: op, Value: value}
}

// marshal encodes leaf rules (or nested groups) into the raw rules[] entries.
func marshal(entries ...any) []json.RawMessage {
	out := make([]json.RawMessage, len(entries))
	for i, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			panic(err)
		}
		out[i] = b
	}
	return out
}

func TestCompileEvaluatesAgainstContacts(t *testing.T) {
	env := testhelper.Setup(t)
	const ann, bob, cara = fixtures.MassiveAnnEmail, fixtures.MassiveBobEmail, fixtures.MassiveCaraEmail
	match := func(g condition.Group) []string { return emailsIn(t, env, fixtures.MassiveID, g) }

	// Empty group matches everyone.
	assert.Equal(t, []string{ann, bob, cara}, match(condition.Group{Kind: "group", Combinator: "and"}))

	// custom:plan = pro -> Ann + Cara
	assert.Equal(t, []string{ann, cara}, match(condition.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("custom:plan", "eq", "pro"))}))

	// and: first_name=Ann AND plan=pro -> just Ann
	assert.Equal(t, []string{ann}, match(condition.Group{
		Kind:       "group",
		Combinator: "and",
		Rules:      marshal(rule("first_name", "eq", "Ann"), rule("custom:plan", "eq", "pro")),
	}))

	// or: first_name=Ann OR first_name=Bob
	assert.Equal(t, []string{ann, bob}, match(condition.Group{
		Kind:       "group",
		Combinator: "or",
		Rules:      marshal(rule("first_name", "eq", "Ann"), rule("first_name", "eq", "Bob")),
	}))

	// custom:plan notNull -> all have it
	assert.Equal(t, []string{ann, bob, cara}, match(condition.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("custom:plan", "notNull", ""))}))

	// not: NOT(plan=pro) -> only Bob (free)
	assert.Equal(t, []string{bob}, match(condition.Group{Kind: "group", Combinator: "and", Not: true, Rules: marshal(rule("custom:plan", "eq", "pro"))}))
}

// eventLeaf is an event count leaf; withinDays 0 leaves the window out (all time).
func eventLeaf(action, op string, times, withinDays int) condition.EventLeaf {
	l := condition.EventLeaf{Kind: "event", Action: action, Operator: op, Times: &times}
	if withinDays != 0 {
		l.WithinDays = &withinDays
	}
	return l
}

// Massive's Events: Ann did page_view 2, 3 and 20 days ago and purchase 40 days ago; one
// page_view is anonymous; Bob opened an email once by hand and twice by software, Cara twice by hand.
func TestEventConditions(t *testing.T) {
	env := testhelper.Setup(t)
	const ann, bob, cara = fixtures.MassiveAnnEmail, fixtures.MassiveBobEmail, fixtures.MassiveCaraEmail

	for name, tc := range map[string]struct {
		leaf condition.EventLeaf
		want []string
	}{
		"performed ever":                            {eventLeaf("page_view", "atLeast", 1, 0), []string{ann}},
		"performed within 7 days":                   {eventLeaf("page_view", "atLeast", 1, 7), []string{ann}},
		"performed long ago, not within the window": {eventLeaf("purchase", "atLeast", 1, 7), nil},
		"performed once ever":                       {eventLeaf("purchase", "atLeast", 1, 0), []string{ann}},
		"at least as many as there are":             {eventLeaf("page_view", "atLeast", 3, 0), []string{ann}},
		"at least more than there are":              {eventLeaf("page_view", "atLeast", 4, 0), nil},
		"at least two within a week":                {eventLeaf("page_view", "atLeast", 2, 7), []string{ann}},
		"the third is older than the week":          {eventLeaf("page_view", "atLeast", 3, 7), nil},
		// An anonymous page_view (no Contact yet) must not hide Bob and Cara: a NOT IN over a NULL
		// contact_id would match nobody.
		"not performed":                  {eventLeaf("page_view", "atMost", 0, 0), []string{bob, cara}},
		"at most fewer than Ann has":     {eventLeaf("page_view", "atMost", 2, 0), []string{bob, cara}},
		"at most as many as in a week":   {eventLeaf("page_view", "atMost", 2, 7), []string{ann, bob, cara}},
		"a window past any Event":        {eventLeaf("page_view", "atLeast", 3, math.MaxInt32), []string{ann}},
		"software opens are not counted": {eventLeaf("email.opened", "atLeast", 2, 0), []string{cara}},
		"at most one human open":         {eventLeaf("email.opened", "atMost", 1, 0), []string{ann, bob}},
	} {
		got := emailsIn(t, env, fixtures.MassiveID, opsGroup(tc.leaf))
		assert.ElementsMatch(t, tc.want, got, name)
	}
}

// An event leaf composes with the other leaves and groups of the tree.
func TestEventConditionsCompose(t *testing.T) {
	env := testhelper.Setup(t)

	exactlyOne := opsGroup(eventLeaf("email.opened", "atLeast", 1, 0), eventLeaf("email.opened", "atMost", 1, 0))
	got := emailsIn(t, env, fixtures.MassiveID, exactlyOne)

	assert.Equal(t, []string{fixtures.MassiveBobEmail}, got, "exactly once is a group of atLeast and atMost")
}

// Fixtures: tag "vip" is on contacts 1 and 3, "newsletter" on contact 1, "unused" on
// nobody. Membership is live: applying or removing a Tag moves a Contact at once.
func TestTagConditions(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// The anchor contacts 1-3 are the only ones the fixtures tag.
	count := func(g condition.Group) int {
		p, err := condition.Compile(ctx, env.DB.Scoped(fixtures.AcmeID), g)
		require.NoError(t, err)
		n, err := env.DB.Contact.Query().
			Where(contact.WorkspaceID(fixtures.AcmeID), contact.IDIn(fixtures.ContactAliceID, fixtures.ContactBobID, fixtures.ContactCarolID), p).
			Count(ctx)
		require.NoError(t, err)
		return n
	}
	has := func(name string) condition.Group {
		return condition.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("tag", "has", name))}
	}

	assert.Equal(t, 2, count(has("vip")))
	assert.Equal(t, 1, count(has("newsletter")))
	assert.Equal(t, 0, count(has("unused")), "an existing Tag nobody has matches no one")
	assert.Equal(t, 0, count(has("never-created")))
	assert.Equal(t, 1, count(condition.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("tag", "doesNotHave", "vip"))}), "contact 2")

	// Composes with other rules: vip AND newsletter -> contact 1 only.
	assert.Equal(t, 1, count(condition.Group{
		Kind:       "group",
		Combinator: "and",
		Rules:      marshal(rule("tag", "has", "vip"), rule("tag", "has", "newsletter")),
	}))

	// Live: tagging contact 2 adds it to the audience.
	_, err := contacts.New(env.Bus).ApplyTag(ctx, env.DB.Scoped(fixtures.AcmeID), 2, "vip")
	require.NoError(t, err)
	assert.Equal(t, 3, count(has("vip")))

	// A same-named Tag in another workspace never matches here.
	assert.Equal(t, 0, count(has(fixtures.TagGlobexName)))

	// Validation: only has / doesNotHave are valid for the tag field.
	assert.Error(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"tag","operator":"eq","value":"vip"}]}`))
}

func TestParseAndValidate(t *testing.T) {
	env := testhelper.Setup(t)
	g, err := condition.Parse("")
	require.NoError(t, err)
	assert.Equal(t, "and", g.Combinator)

	require.NoError(t, validate(t, env, `{"kind":"group","combinator":"or","rules":[{"kind":"rule","field":"email","operator":"contains","value":"x"}]}`))

	// nested group
	require.NoError(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"group","combinator":"or","rules":[{"kind":"rule","field":"first_name","operator":"eq","value":"Ann"}]}]}`))

	// unknown field
	require.Error(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`))
	// unsupported operator
	require.Error(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"email","operator":"weird","value":"x"}]}`))
	// malformed JSON
	assert.Error(t, validate(t, env, `{not json`))
}

// Fixtures: Liam (100) has a Binding of his own, Emma (101) one through her phone Visitor, Alice
// (1) none; Globex's Contact has one in its own Workspace. "has" is true for a Binding of the
// Contact or of a Visitor of it, "doesNotHave" is the complement, live.
func TestTelegramConditions(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	const liam, emma, alice = fixtures.ContactLiamID, 101, fixtures.ContactAliceID

	matching := func(op string) []int64 {
		g := condition.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("telegram", op, ""))}
		p, err := condition.Compile(ctx, env.DB.Scoped(fixtures.AcmeID), g)
		require.NoError(t, err)
		ids, err := env.DB.Contact.Query().
			Where(contact.WorkspaceID(fixtures.AcmeID), contact.IDIn(liam, emma, alice), p).
			IDs(ctx)
		require.NoError(t, err)
		return ids
	}

	assert.ElementsMatch(t, []int64{liam, emma}, matching("has"), "own Binding and a Visitor's")
	assert.ElementsMatch(t, []int64{alice}, matching("doesNotHave"))

	// Live: unbinding Liam moves him to the complement.
	env.DB.Binding.DeleteOneID(fixtures.BindingLiamID).ExecX(ctx)
	assert.ElementsMatch(t, []int64{emma}, matching("has"))
	assert.ElementsMatch(t, []int64{liam, alice}, matching("doesNotHave"))

	// Only has / doesNotHave are valid for the telegram field.
	require.Error(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"telegram","operator":"eq","value":""}]}`))
	require.NoError(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"telegram","operator":"has"}]}`))
}

// The sms field selects the Contacts that can be texted: a phone with an SMS Confirmation, no SMS
// opt-out and no Suppression, Send-eligibility on sms read live. Fixtures (the eligibility matrix of
// Tyrell, confirmed opt-in off, and Lumon, on): "clean" has a phone and no Confirmation,
// "unsubscribed from everything" a phone with a Confirmation and an SMS opt-out, "confirmed" a phone
// with a Confirmation; the others have no phone. Lumon's email gate changes nothing on sms.
func TestSMSConditions(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	matching := func(ws int64, op string) []int64 {
		t.Helper()
		g := condition.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("sms", op, ""))}
		s := env.DB.Scoped(ws)
		p, err := condition.Compile(ctx, s, g)
		require.NoError(t, err)
		ids, err := s.Contact().Query().Where(p).IDs(ctx)
		require.NoError(t, err)
		return ids
	}
	everyone := func(ws int64) []int64 {
		t.Helper()
		ids, err := env.DB.Scoped(ws).Contact().Query().IDs(ctx)
		require.NoError(t, err)
		return ids
	}

	for _, ws := range []struct {
		id                                int64
		clean, unsubEverything, confirmed int64
		phoneless                         []int64
	}{
		{fixtures.TyrellID, fixtures.TyrellMatrixCleanID, fixtures.TyrellMatrixUnsubEverythingID, fixtures.TyrellMatrixConfirmedID,
			[]int64{fixtures.TyrellMatrixSuppressedID, fixtures.TyrellMatrixUnsubBroadcastsID, fixtures.TyrellMatrixConfirmedSuppressedID}},
		{fixtures.LumonID, fixtures.LumonMatrixCleanID, fixtures.LumonMatrixUnsubEverythingID, fixtures.LumonMatrixConfirmedID,
			[]int64{fixtures.LumonMatrixSuppressedID, fixtures.LumonMatrixUnsubBroadcastsID, fixtures.LumonMatrixConfirmedSuppressedID}},
	} {
		has, hasNot := matching(ws.id, "has"), matching(ws.id, "doesNotHave")
		assert.Equal(t, []int64{ws.confirmed}, has, "workspace %d", ws.id)
		assert.Subset(t, hasNot, append([]int64{ws.clean, ws.unsubEverything}, ws.phoneless...), "workspace %d", ws.id)
		assert.ElementsMatch(t, everyone(ws.id), append(has, hasNot...), "the complement, workspace %d", ws.id)
	}

	// Live: confirming the clean phone moves it in, opting the confirmed one out moves it out.
	tyrell := env.DB.Scoped(fixtures.TyrellID)
	_, err := confirmations.ConfirmContactSMS(ctx, env.Bus, tyrell, fixtures.TyrellMatrixCleanID,
		confirmations.Evidence{Wording: "Text me offers.", Source: "test", Brand: "Tyrell"})
	require.NoError(t, err)
	_, err = optout.OptOutContactSMS(ctx, env.Bus, tyrell, fixtures.TyrellMatrixConfirmedID)
	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.TyrellMatrixCleanID}, matching(fixtures.TyrellID, "has"))
	assert.Contains(t, matching(fixtures.TyrellID, "doesNotHave"), int64(fixtures.TyrellMatrixConfirmedID))

	// Only has / doesNotHave are valid for the sms field.
	require.Error(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"sms","operator":"eq","value":""}]}`))
	require.NoError(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"sms","operator":"doesNotHave"}]}`))
}
