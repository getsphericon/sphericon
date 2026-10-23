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
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func acme(env *testhelper.TestEnv) *ent.Scoped { return env.DB.Scoped(fixtures.AcmeID) }

func TestArchiveHidesARuleFromTheListAndStopsItFiring(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)

	require.NoError(t, m.Archive(ctx, acme(env), fixtures.RuleTagClickedID))

	kept, err := m.List(ctx, acme(env), pagination.Params{Page: 1, PageSize: 100}, false)
	require.NoError(t, err)
	for _, r := range kept.Items {
		assert.NotEqual(t, int64(fixtures.RuleTagClickedID), r.ID)
	}
	archived, err := m.List(ctx, acme(env), pagination.Params{Page: 1, PageSize: 100}, true)
	require.NoError(t, err)
	require.Len(t, archived.Items, 1)
	assert.Equal(t, rule.StatusDraft, archived.Items[0].Status, "an archived Rule is no longer active")
	assert.NotNil(t, archived.Items[0].DiscardedAt)
	assert.Empty(t, evaluate(t, env, fixtures.ContactAliceID, "rule_click", "evt-after-archive"))
	got, err := m.Get(ctx, acme(env), fixtures.RuleTagClickedID)
	require.NoError(t, err, "an archived Rule is still addressable")
	assert.NotNil(t, got.DiscardedAt)
}

func TestUnarchiveBringsARuleBackAsADraft(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	require.NoError(t, m.Archive(ctx, acme(env), fixtures.RuleTagClickedID))

	require.NoError(t, m.Unarchive(ctx, acme(env), fixtures.RuleTagClickedID))

	got, err := m.Get(ctx, acme(env), fixtures.RuleTagClickedID)
	require.NoError(t, err)
	assert.Nil(t, got.DiscardedAt)
	assert.Equal(t, rule.StatusDraft, got.Status)
}

func TestArchiveOfAnotherWorkspacesRuleIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)

	assert.ErrorIs(t, rules.New(env.Cipher, env.Bus).Archive(context.Background(), acme(env), fixtures.RuleGlobexID), rules.ErrNotFound)
}

func TestArchivingAnArchivedRuleChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	require.NoError(t, m.Archive(ctx, acme(env), fixtures.RuleDraftID))

	require.NoError(t, m.Archive(ctx, acme(env), fixtures.RuleDraftID))

	got, err := m.Get(ctx, acme(env), fixtures.RuleDraftID)
	require.NoError(t, err)
	assert.NotNil(t, got.DiscardedAt)
}

// Activating and archiving a Rule at the same moment leave it archived and a draft, or active
// and kept; never archived and active (an archived Rule must act on nothing).
func TestActivatingAndArchivingARuleAtOnceNeverLeaveItArchivedAndActive(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	sqlDB.SetMaxOpenConns(8)
	client := db.NewEntClient(sqlDB)
	m := rules.New(nil, events.New(sqlDB))
	s := client.Scoped(fixtures.AcmeID)

	// Either may be refused because the other won; what matters is the state they leave.
	var activateErr, archiveErr error
	var wg sync.WaitGroup
	wg.Go(func() { _, activateErr = m.Activate(t.Context(), s, fixtures.RuleDraftID) })
	wg.Go(func() { archiveErr = m.Archive(t.Context(), s, fixtures.RuleDraftID) })
	wg.Wait()

	got, err := m.Get(t.Context(), s, fixtures.RuleDraftID)
	require.NoError(t, err)
	assert.False(t, got.DiscardedAt != nil && got.Status == rule.StatusActive, "archived and active (activate: %v, archive: %v)", activateErr, archiveErr)
}

func TestDeletePurgesARuleWithNoFirings(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)

	require.NoError(t, m.Delete(ctx, acme(env), fixtures.RuleDraftID))

	_, err := m.Get(ctx, acme(env), fixtures.RuleDraftID)
	assert.ErrorIs(t, err, rules.ErrNotFound)
}

func TestDeleteRefusesARuleWithFiringsAndNamesTheirCount(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus)

	err := m.Delete(context.Background(), acme(env), fixtures.RuleTagClickedID)

	refused, ok := inuse.As(err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, inuse.SubjectRule, refused.What)
	require.Len(t, refused.Users, 1)
	assert.Equal(t, inuse.Firing, refused.Users[0].Kind)
	assert.Equal(t, 2, refused.Users[0].Count)
	_, err = m.Get(context.Background(), acme(env), fixtures.RuleTagClickedID)
	require.NoError(t, err, "the Rule stays")
}

func TestDeleteOfAnotherWorkspacesRuleIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)

	assert.ErrorIs(t, rules.New(env.Cipher, env.Bus).Delete(context.Background(), acme(env), fixtures.RuleGlobexID), rules.ErrNotFound)
}

func tagAction(tag string) []rules.Action {
	return []rules.Action{{Type: rules.ActionApplyTag, Tag: tag}}
}

func TestAnActiveRuleThatAppliesATagUsesIt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	r, err := m.Create(ctx, acme(env), rules.CreateInput{Name: "uses unused", TriggerEvents: []string{"x"}, Actions: tagAction("unused")})
	require.NoError(t, err)

	draft, err := rules.UsingTag(ctx, acme(env), "unused")
	require.NoError(t, err)
	assert.Empty(t, draft, "a draft acts on nothing, so it uses nothing")

	_, err = m.Activate(ctx, acme(env), r.ID)
	require.NoError(t, err)
	active, err := rules.UsingTag(ctx, acme(env), "unused")
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, r.ID, active[0].ID)
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

func TestAnArchivedRuleUsesNoTag(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	require.NoError(t, m.Archive(ctx, acme(env), fixtures.RuleVipOnlyID))
	require.NoError(t, m.Archive(ctx, acme(env), fixtures.RuleMultiTriggerID))

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

	_, err = m.Update(ctx, acme(env), fixtures.RuleDraftID, rules.UpdateInput{Condition: &big})
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

// Create, update, activate and archive each leave an Audit entry, and the Actions' values
// never reach one (a later Action kind may hold a secret).
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
	_, err = m.Activate(ctx, s, r.ID)
	require.NoError(t, err)
	require.NoError(t, m.Archive(ctx, s, r.ID))

	entries := lo.FilterMap(env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) (*events.AuditEntry, bool) {
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
	assert.Contains(t, seen, "rule.discard")
	assert.GreaterOrEqual(t, len(seen), 4, "update, activate and archive each wrote one")
}
