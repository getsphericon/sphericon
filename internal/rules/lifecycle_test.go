package rules_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func acme(env *testhelper.TestEnv) *ent.Scoped { return env.DB.Scoped(fixtures.AcmeID) }

// A Rule with Firings is discarded by Delete: it leaves the list and stops firing, and it is still
// read by id, inactive and marked deleted, with its Firings (ADR 0049, ADR 0098).
func TestDeleteOfARuleWithFiringsHidesItStopsItFiringAndKeepsTheFirings(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	page := pagination.Params{Page: 1, PageSize: 100}
	before, err := m.Firings(ctx, acme(env), fixtures.RuleTagClickedID, page)
	require.NoError(t, err)
	require.NotEmpty(t, before.Items)

	require.NoError(t, m.Delete(ctx, acme(env), fixtures.RuleTagClickedID))

	kept, err := m.List(ctx, acme(env), page)
	require.NoError(t, err)
	assert.NotContains(t, lo.Map(kept.Items, func(r *ent.Rule, _ int) int64 { return r.ID }), int64(fixtures.RuleTagClickedID))
	assert.Empty(t, evaluate(t, env, fixtures.ContactAliceID, "rule_click", "evt-after-delete"))
	got, err := m.Get(ctx, acme(env), fixtures.RuleTagClickedID)
	require.NoError(t, err, "a deleted Rule is still read by id")
	assert.NotNil(t, got.DiscardedAt)
	assert.Equal(t, rule.StatusInactive, got.Status)
	after, err := m.Firings(ctx, acme(env), fixtures.RuleTagClickedID, page)
	require.NoError(t, err)
	assert.Equal(t, before.TotalItems, after.TotalItems, "its Firings stay readable")
}

// A deleted Rule changes no more: every move but a second Delete is not found, and that one
// changes nothing.
func TestADeletedRuleTakesNoMove(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	s := acme(env)
	id := int64(fixtures.RuleTagClickedID)
	require.NoError(t, m.Delete(ctx, s, id))

	name := "renamed"
	moves := map[string]func() error{
		"update":     func() error { _, err := m.Update(ctx, s, id, rules.UpdateInput{Name: &name}); return err },
		"publish":    func() error { _, err := m.Publish(ctx, s, id); return err },
		"revert":     func() error { _, err := m.Revert(ctx, s, id); return err },
		"deactivate": func() error { _, err := m.Deactivate(ctx, s, id); return err },
		"dry run":    func() error { _, err := m.DryRun(ctx, s, id, 999999); return err },
	}
	for move, run := range moves {
		require.ErrorIs(t, run(), rules.ErrNotFound, move)
	}
	require.NoError(t, m.Delete(ctx, s, id), "deleting a deleted Rule changes nothing")
	got, err := m.Get(ctx, s, id)
	require.NoError(t, err)
	assert.Equal(t, fixtures.RuleTagClickedName, got.Name)
}

// Publishing and deleting a fired Rule at the same moment leave it deleted and inactive, or active
// and kept; never deleted and active (a deleted Rule must act on nothing).
func TestPublishingAndDeletingARuleAtOnceNeverLeaveItDeletedAndActive(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	sqlDB.SetMaxOpenConns(8)
	client := db.NewEntClient(sqlDB)
	m := rules.New(nil, events.New(sqlDB))
	s := client.Scoped(fixtures.AcmeID)
	_, err := m.Deactivate(t.Context(), s, fixtures.RuleTagClickedID)
	require.NoError(t, err)

	// Either may be refused because the other won; what matters is the state they leave.
	var publishErr, deleteErr error
	var wg sync.WaitGroup
	wg.Go(func() { _, publishErr = m.Publish(t.Context(), s, fixtures.RuleTagClickedID) })
	wg.Go(func() { deleteErr = m.Delete(t.Context(), s, fixtures.RuleTagClickedID) })
	wg.Wait()

	got, err := m.Get(t.Context(), s, fixtures.RuleTagClickedID)
	require.NoError(t, err)
	assert.False(t, got.DiscardedAt != nil && got.Status == rule.StatusActive, "deleted and active (publish: %v, delete: %v)", publishErr, deleteErr)
}

// Delete discards a Rule that never fired too: there is no purge for a row without history yet
// (ADR 0100). It is still read by id, and its versions with it.
func TestDeleteKeepsARuleThatNeverFired(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)

	require.NoError(t, m.Delete(ctx, acme(env), fixtures.RuleInactiveID))

	got, err := m.Get(ctx, acme(env), fixtures.RuleInactiveID)
	require.NoError(t, err, "a deleted Rule that never fired is still read by id")
	assert.NotNil(t, got.DiscardedAt)
	live, draft := rules.LiveAndDraft(got)
	assert.True(t, live != nil || draft != nil, "its versions are kept with it")
}

func TestDeleteOfAnotherWorkspacesRuleIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)

	assert.ErrorIs(t, rules.New(env.Cipher, env.Bus).Delete(context.Background(), acme(env), fixtures.RuleGlobexID), rules.ErrNotFound)
}

func tagAction(tag string) []rules.Action {
	return []rules.Action{{Type: rules.ActionApplyTag, Tag: tag}}
}

// A Rule's draft names its Tag even before it is published (spec #767, "In use"): the Rule never
// published names rule-inactive, and still does once it is.
func TestARuleThatAppliesATagUsesItBeforeAndAfterPublish(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)

	drafted, err := rules.UsingTag(ctx, acme(env), "rule-inactive")
	require.NoError(t, err)
	_, err = m.Publish(ctx, acme(env), fixtures.RuleInactiveID)
	require.NoError(t, err)
	published, err := rules.UsingTag(ctx, acme(env), "rule-inactive")
	require.NoError(t, err)

	assert.Equal(t, []int64{fixtures.RuleInactiveID}, lo.Map(drafted, func(r *ent.Rule, _ int) int64 { return r.ID }))
	assert.Equal(t, []int64{fixtures.RuleInactiveID}, lo.Map(published, func(r *ent.Rule, _ int) int64 { return r.ID }))
}

func TestAnActiveRuleWhoseConditionNamesATagUsesIt(t *testing.T) {
	env := testhelper.Setup(t)

	// RuleVipOnly is active and its Condition tests the vip Tag; RuleMultiTrigger removes it.
	using, err := rules.UsingTag(context.Background(), acme(env), "vip")

	require.NoError(t, err)
	ids := make([]int64, 0, len(using))
	for _, r := range using {
		ids = append(ids, r.ID)
	}
	assert.Equal(t, []int64{fixtures.RuleVipOnlyID, fixtures.RuleMultiTriggerID}, ids)
}

func TestADeletedRuleUsesNoTag(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	require.NoError(t, m.Delete(ctx, acme(env), fixtures.RuleVipOnlyID))
	require.NoError(t, m.Delete(ctx, acme(env), fixtures.RuleMultiTriggerID))

	using, err := rules.UsingTag(ctx, acme(env), "vip")

	require.NoError(t, err)
	assert.Empty(t, using)
}

func manyActions(n int) []rules.Action {
	actions := make([]rules.Action, n)
	for i := range actions {
		actions[i] = rules.Action{Type: rules.ActionApplyTag, Tag: "t" + strconv.Itoa(i)}
	}
	return actions
}

func TestCreateRefusesMoreActionsThanTheCap(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := rules.New(env.Cipher, env.Bus).Create(context.Background(), acme(env),
		rules.CreateInput{Name: "big", TriggerEvents: []string{"x"}, Actions: manyActions(rules.MaxActions + 1)})

	require.ErrorIs(t, err, rules.ErrInvalid)
	assert.Contains(t, err.Error(), fmt.Sprintf("at most %d Actions", rules.MaxActions))
}

func TestCreateAcceptsTheCapExactlyAndUpdateEnforcesIt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)

	r, err := m.Create(ctx, acme(env), rules.CreateInput{Name: "full", TriggerEvents: []string{"x"}, Actions: manyActions(rules.MaxActions)})
	require.NoError(t, err)

	over := manyActions(rules.MaxActions + 1)
	_, err = m.Update(ctx, acme(env), r.ID, rules.UpdateInput{Actions: &over})
	require.ErrorIs(t, err, rules.ErrInvalid)
}

// conditionOf is an `and` group of n tag leaves: n+1 nodes counting the group.
func conditionOf(n int) string {
	leaf := `{"kind":"rule","field":"tag","operator":"has","value":"vip"}`
	return `{"kind":"group","combinator":"and","rules":[` + strings.TrimSuffix(strings.Repeat(leaf+",", n), ",") + `]}`
}

func TestARuleRefusesAConditionLargerThanTheCap(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	big := conditionOf(rules.MaxConditionNodes)

	_, err := m.Create(ctx, acme(env), rules.CreateInput{Name: "big", TriggerEvents: []string{"x"}, Condition: &big})
	require.ErrorIs(t, err, rules.ErrInvalid)
	assert.Contains(t, err.Error(), "Condition")

	_, err = m.Update(ctx, acme(env), fixtures.RuleInactiveID, rules.UpdateInput{Condition: &big})
	require.ErrorIs(t, err, rules.ErrInvalid)

	fits := conditionOf(rules.MaxConditionNodes - 1)
	_, err = m.Create(ctx, acme(env), rules.CreateInput{Name: "fits", TriggerEvents: []string{"x"}, Condition: &fits})
	require.NoError(t, err)
}

func TestARuleHasNoCapOnTheirNumber(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus)
	for i := range 30 {
		_, err := m.Create(context.Background(), acme(env), rules.CreateInput{Name: "r" + strconv.Itoa(i), TriggerEvents: []string{"x"}, Actions: tagAction("a")})
		require.NoError(t, err)
	}
}

// Create, update, publish, deactivate and delete each leave an Audit entry, and the Actions' values
// never reach one (a later Action kind may hold a secret). Delete records a discard (ADR 0100).
func TestRuleChangesAreAudited(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	actor := events.Actor{Kind: events.ActorUser, ID: strconv.Itoa(fixtures.OwnerJohnID), Name: "John"}
	s := env.Bus.Act(acme(env), actor)
	m := rules.New(env.Cipher, env.Bus)

	r, err := m.Create(ctx, s, rules.CreateInput{Name: "audited", TriggerEvents: []string{"x"}, Actions: tagAction("secretish")})
	require.NoError(t, err)
	acts := tagAction("secretish2")
	_, err = m.Update(ctx, s, r.ID, rules.UpdateInput{Actions: &acts})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, r.ID)
	require.NoError(t, err)
	_, err = m.Deactivate(ctx, s, r.ID)
	require.NoError(t, err)
	require.NoError(t, m.Delete(ctx, s, r.ID))

	entries := lo.FilterMap(env.QueuedEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) (*events.AuditEntry, bool) {
		e, ok := ev.(*events.AuditEntry)
		return e, ok && e.TargetType == "rule" && e.TargetID == strconv.FormatInt(r.ID, 10)
	})
	seen := lo.Map(entries, func(e *events.AuditEntry, _ int) string { return e.Action })
	for _, e := range entries {
		assert.Equal(t, actor, e.Actor)
		assert.NotContains(t, fmt.Sprint(e.Diff), "secretish")
	}
	assert.Equal(t, "rule.create", seen[0])
	assert.Contains(t, seen, "rule.update")
	assert.Equal(t, "rule.discard", seen[len(seen)-1])
	assert.GreaterOrEqual(t, len(seen), 5, "update, publish, deactivate and delete each wrote one")
}
