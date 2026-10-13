package segments_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/tags"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validate runs the Module's rule set for the Acme Workspace.
func validate(t *testing.T, env *testhelper.TestEnv, def string) error {
	t.Helper()
	return segments.New().Validate(context.Background(), env.DB.Scoped(fixtures.AcmeID), def)
}

func rule(field, op, value string) segments.Rule {
	return segments.Rule{Kind: "rule", Field: field, Operator: op, Value: value}
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
	ctx := context.Background()

	mk := func(email, first string, custom map[string]any) {
		c := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail(email).SetFirstName(first)
		if custom != nil {
			c.SetCustomFields(custom)
		}
		_, err := c.Save(ctx)
		require.NoError(t, err)
	}
	mk("ann@seg.test", "Ann", map[string]any{"plan": "pro"})
	mk("bob@seg.test", "Bob", map[string]any{"plan": "free"})
	mk("cara@seg.test", "Cara", map[string]any{"plan": "pro"})

	count := func(g segments.Group) int {
		p, err := segments.Compile(ctx, env.DB.Scoped(fixtures.AcmeID), g)
		require.NoError(t, err)
		n, err := env.DB.Contact.Query().
			Where(contact.WorkspaceID(fixtures.AcmeID), contact.EmailContainsFold("@seg.test"), predicate.Contact(p)).
			Count(ctx)
		require.NoError(t, err)
		return n
	}

	// Empty group matches everyone (within the @seg.test scope: all 3).
	assert.Equal(t, 3, count(segments.Group{Kind: "group", Combinator: "and"}))

	// custom:plan = pro → Ann + Cara
	assert.Equal(t, 2, count(segments.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("custom:plan", "eq", "pro"))}))

	// and: first_name=Ann AND plan=pro → just Ann
	assert.Equal(t, 1, count(segments.Group{
		Kind:       "group",
		Combinator: "and",
		Rules:      marshal(rule("first_name", "eq", "Ann"), rule("custom:plan", "eq", "pro")),
	}))

	// or: first_name=Ann OR first_name=Bob → 2
	assert.Equal(t, 2, count(segments.Group{
		Kind:       "group",
		Combinator: "or",
		Rules:      marshal(rule("first_name", "eq", "Ann"), rule("first_name", "eq", "Bob")),
	}))

	// custom:plan notNull → all 3 have it
	assert.Equal(t, 3, count(segments.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("custom:plan", "notNull", ""))}))

	// not: NOT(plan=pro) → only Bob (free)
	assert.Equal(t, 1, count(segments.Group{Kind: "group", Combinator: "and", Not: true, Rules: marshal(rule("custom:plan", "eq", "pro"))}))
}

func TestEventConditions(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	mk := func(email string) int64 {
		c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail(email).Save(ctx)
		require.NoError(t, err)
		return c.ID
	}
	// Events attach by the stable contact_id (ADR 0002), not the email string.
	mkEvent := func(contactID int64, action string, workspace int64, occurred time.Time) {
		_, err := env.DB.Event.Create().
			SetWorkspaceID(workspace).SetContactID(contactID).
			SetAction(action).SetOccurredAt(occurred).Save(ctx)
		require.NoError(t, err)
	}
	annID := mk("ann@evseg.test")
	mk("bob@evseg.test")

	now := time.Now()
	mkEvent(annID, "page_view", fixtures.AcmeID, now.AddDate(0, 0, -2)) // recent
	mkEvent(annID, "purchase", fixtures.AcmeID, now.AddDate(0, 0, -40)) // old
	// An anonymous event of the same action (no Contact yet) must not hide Bob from
	// notPerformed: NOT IN over a NULL contact_id would match nobody.
	_, err := env.DB.Event.Create().SetWorkspaceID(fixtures.AcmeID).SetAction("page_view").
		SetVisitorID("anon-visitor").SetOccurredAt(now).Save(ctx)
	require.NoError(t, err)

	count := func(g segments.Group) int {
		p, err := segments.Compile(ctx, env.DB.Scoped(fixtures.AcmeID), g)
		require.NoError(t, err)
		n, err := env.DB.Contact.Query().
			Where(contact.WorkspaceID(fixtures.AcmeID), contact.EmailContainsFold("@evseg.test"), predicate.Contact(p)).
			Count(ctx)
		require.NoError(t, err)
		return n
	}

	// performed ever → only Ann did page_view.
	assert.Equal(t, 1, count(segments.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("event:page_view", "performed", ""))}))
	// performed within 7 days → Ann's page_view (2d ago) qualifies.
	assert.Equal(t, 1, count(segments.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("event:page_view", "performed", "7"))}))
	// purchase within 7 days → none (Ann's is 40d ago); ever → Ann.
	assert.Equal(t, 0, count(segments.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("event:purchase", "performed", "7"))}))
	assert.Equal(t, 1, count(segments.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("event:purchase", "performed", ""))}))
	// notPerformed page_view → Bob only.
	assert.Equal(t, 1, count(segments.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("event:page_view", "notPerformed", ""))}))

	// validation: bad operator + non-numeric window are rejected.
	assert.Error(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"event:x","operator":"weird","value":""}]}`))
	assert.Error(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"event:x","operator":"performed","value":"soon"}]}`))
}

// Fixtures: tag "vip" is on contacts 1 and 3, "newsletter" on contact 1, "unused" on
// nobody. Membership is live: applying or removing a Tag moves a Contact at once.
func TestTagConditions(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// The anchor contacts 1-3 are the only ones the fixtures tag.
	count := func(g segments.Group) int {
		p, err := segments.Compile(ctx, env.DB.Scoped(fixtures.AcmeID), g)
		require.NoError(t, err)
		n, err := env.DB.Contact.Query().
			Where(contact.WorkspaceID(fixtures.AcmeID), contact.IDIn(fixtures.ContactAliceID, fixtures.ContactBobID, fixtures.ContactCarolID), predicate.Contact(p)).
			Count(ctx)
		require.NoError(t, err)
		return n
	}
	has := func(name string) segments.Group {
		return segments.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("tag", "has", name))}
	}

	assert.Equal(t, 2, count(has("vip")))
	assert.Equal(t, 1, count(has("newsletter")))
	assert.Equal(t, 0, count(has("unused")), "an existing Tag nobody has matches no one")
	assert.Equal(t, 0, count(has("never-created")))
	assert.Equal(t, 1, count(segments.Group{Kind: "group", Combinator: "and", Rules: marshal(rule("tag", "doesNotHave", "vip"))}), "contact 2")

	// Composes with other rules: vip AND newsletter -> contact 1 only.
	assert.Equal(t, 1, count(segments.Group{
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
	g, err := segments.Parse("")
	require.NoError(t, err)
	assert.Equal(t, "and", g.Combinator)

	require.NoError(t, validate(t, env, `{"kind":"group","combinator":"or","rules":[{"kind":"rule","field":"email","operator":"contains","value":"x"}]}`))

	// nested group
	require.NoError(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"group","combinator":"or","rules":[{"kind":"rule","field":"first_name","operator":"eq","value":"Ann"}]}]}`))

	// unknown field
	assert.Error(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`))
	// unsupported operator
	assert.Error(t, validate(t, env, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"email","operator":"weird","value":"x"}]}`))
	// malformed JSON
	assert.Error(t, validate(t, env, `{not json`))
}
