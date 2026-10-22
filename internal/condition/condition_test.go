package condition_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/tags"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	emails, err := s.Contact().Query().Where(predicate.Contact(p)).Order(contact.ByEmail()).Select(contact.FieldEmail).Strings(ctx)
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

func TestEventConditions(t *testing.T) {
	env := testhelper.Setup(t)
	const ann, bob, cara = fixtures.MassiveAnnEmail, fixtures.MassiveBobEmail, fixtures.MassiveCaraEmail
	match := func(g condition.Group) []string { return emailsIn(t, env, fixtures.MassiveID, g) }
	event := func(action, op, value string) condition.Group {
		return condition.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("event:"+action, op, value))}
	}

	// Ann did page_view 2 days ago and purchase 40 days ago.
	assert.Equal(t, []string{ann}, match(event("page_view", "performed", "")), "performed ever")
	assert.Equal(t, []string{ann}, match(event("page_view", "performed", "7")), "performed within 7 days")
	assert.Empty(t, match(event("purchase", "performed", "7")), "Ann's purchase is 40 days old")
	assert.Equal(t, []string{ann}, match(event("purchase", "performed", "")))
	// An anonymous page_view (no Contact yet) must not hide Bob from notPerformed:
	// NOT IN over a NULL contact_id would match nobody.
	assert.Equal(t, []string{bob, cara}, match(event("page_view", "notPerformed", "")))

	// validation: bad operator + non-numeric window are rejected.
	require.Error(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"event:x","operator":"weird","value":""}]}`))
	assert.Error(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"event:x","operator":"performed","value":"soon"}]}`))
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
			Where(contact.WorkspaceID(fixtures.AcmeID), contact.IDIn(fixtures.ContactAliceID, fixtures.ContactBobID, fixtures.ContactCarolID), predicate.Contact(p)).
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
	_, err := tags.New().Apply(ctx, env.DB.Scoped(fixtures.AcmeID), 2, "vip")
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
