package rules_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/ent/ruleversion"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// appliedTags is the Tags a version's Actions apply, in order.
func appliedTags(t *testing.T, v *ent.RuleVersion) []string {
	t.Helper()
	actions, err := rules.DecodeActions(v.Actions)
	require.NoError(t, err)
	return lo.Map(actions, func(a rules.Action, _ int) string { return a.Tag })
}

func TestCreateStoresTheDefinitionAsTheDraftOfAnInactiveRule(t *testing.T) {
	env := testhelper.Setup(t)

	r, err := rules.New(env.Cipher, env.Bus).Create(context.Background(), acme(env), rules.CreateInput{
		Name: "new", TriggerEvents: []string{"signed_up"}, Actions: tagAction("fresh"),
	})

	require.NoError(t, err)
	live, draft := rules.LiveAndDraft(r)
	assert.Nil(t, live, "a Rule never published has no live version")
	require.NotNil(t, draft)
	assert.Equal(t, 1, draft.Version)
	assert.Equal(t, []string{"signed_up"}, rules.Triggers(draft))
	assert.Equal(t, []string{"fresh"}, appliedTags(t, draft))
	assert.Equal(t, rule.StatusInactive, r.Status)
}

// #748: an edit of an active Rule writes a draft above its live version, and the Rule keeps firing
// on the live one.
func TestUpdatingAnActiveRuleWritesADraftAndLeavesTheLiveVersion(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	triggers := []string{"rule_vip", "rule_vip_again"}
	cond := `{"kind":"group","combinator":"and","rules":[]}`
	actions := tagAction("rule-vip-edited")

	r, err := m.Update(ctx, acme(env), fixtures.RuleVipOnlyID, rules.UpdateInput{TriggerEvents: &triggers, Condition: &cond, Actions: &actions})

	require.NoError(t, err)
	live, draft := rules.LiveAndDraft(r)
	require.NotNil(t, draft)
	assert.Equal(t, 2, draft.Version, "the draft is the version above the newest")
	assert.Equal(t, triggers, rules.Triggers(draft))
	assert.Nil(t, draft.Condition, "an empty group matches every Contact")
	assert.Equal(t, []string{"rule-vip-edited"}, appliedTags(t, draft))
	assert.EqualValues(t, fixtures.RuleVipOnlyID, live.ID, "the live version is untouched")
	assert.Equal(t, []string{"rule-vip-seen"}, appliedTags(t, live))
	assert.Equal(t, rule.StatusActive, r.Status)

	// Alice has the vip Tag and Bob has not: the live Condition still decides, and the draft's
	// Trigger event fires nothing.
	fired, err := m.Evaluate(ctx, acme(env), rules.Event{ContactID: fixtures.ContactAliceID, Action: "rule_vip", SourceID: "evt-748", RecordedAt: time.Now()})
	require.NoError(t, err)
	require.Len(t, fired, 1)
	assert.EqualValues(t, fixtures.RuleVipOnlyID, fired[0].VersionID, "the Firing stands on the live version")
	runs, err := rules.FiringActions(ctx, acme(env), fired[0])
	require.NoError(t, err)
	assert.Equal(t, "rule-vip-seen", runs[0].Tag)
	bob, err := m.Evaluate(ctx, acme(env), rules.Event{ContactID: fixtures.ContactBobID, Action: "rule_vip", SourceID: "evt-748-bob", RecordedAt: time.Now()})
	require.NoError(t, err)
	assert.Empty(t, bob, "the draft's empty Condition is not live")
	again, err := m.Evaluate(ctx, acme(env), rules.Event{ContactID: fixtures.ContactAliceID, Action: "rule_vip_again", SourceID: "evt-748-again", RecordedAt: time.Now()})
	require.NoError(t, err)
	assert.Empty(t, again, "the draft's Trigger event is not live")
}

func TestASecondEditRewritesTheDraftInPlace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	actions := tagAction("rule-clicked-v3")

	r, err := m.Update(ctx, acme(env), fixtures.RuleTagClickedID, rules.UpdateInput{Actions: &actions})

	require.NoError(t, err)
	_, draft := rules.LiveAndDraft(r)
	assert.EqualValues(t, fixtures.RuleTagClickedDraftVersionID, draft.ID)
	assert.Equal(t, 2, draft.Version)
	assert.Equal(t, []string{"rule-clicked-v3"}, appliedTags(t, draft))
	assert.Equal(t, []string{"rule_click"}, rules.Triggers(draft), "a field left out keeps the draft's")
	versions, err := acme(env).RuleVersion().Query().Where(ruleversion.RuleID(fixtures.RuleTagClickedID)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, versions, "no third version")
}

// The editor always sends every field; saving it unchanged, or renaming the Rule, makes no draft.
func TestAnUnchangedDefinitionMakesNoDraft(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	name := "Renamed"
	triggers := []string{"rule_vip"}
	cond := `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"tag","operator":"has","value":"vip"}]}`
	actions := tagAction("rule-vip-seen")

	r, err := m.Update(ctx, acme(env), fixtures.RuleVipOnlyID, rules.UpdateInput{Name: &name, TriggerEvents: &triggers, Condition: &cond, Actions: &actions})

	require.NoError(t, err)
	_, draft := rules.LiveAndDraft(r)
	assert.Nil(t, draft)
	assert.Equal(t, "Renamed", r.Name)
}

func TestAnEmptyConditionGroupIsNoChangeToAVersionWithNoCondition(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	empty := `{"kind":"group","combinator":"and","rules":[]}`
	triggers := []string{"rule_multi_a", "rule_multi_b"}

	r, err := m.Update(ctx, acme(env), fixtures.RuleMultiTriggerID, rules.UpdateInput{TriggerEvents: &triggers, Condition: &empty})

	require.NoError(t, err)
	_, draft := rules.LiveAndDraft(r)
	assert.Nil(t, draft)
}

func TestPublishMakesTheDraftLiveAndKeepsTheOldLiveVersion(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	before, err := m.Get(ctx, acme(env), fixtures.RuleTagClickedID)
	require.NoError(t, err)

	r, err := m.Publish(ctx, acme(env), fixtures.RuleTagClickedID)

	require.NoError(t, err)
	live, draft := rules.LiveAndDraft(r)
	assert.EqualValues(t, fixtures.RuleTagClickedDraftVersionID, live.ID)
	assert.Nil(t, draft)
	assert.Equal(t, before.ActivatedAt, r.ActivatedAt, "republishing an active Rule keeps its activation time")
	old, err := acme(env).RuleVersion().Get(ctx, fixtures.RuleTagClickedLiveVersionID)
	require.NoError(t, err, "the old live version stays as history")
	assert.Equal(t, []string{"rule-clicked"}, appliedTags(t, old))
}

func TestPublishOfARuleWithOnlyALiveVersionActivatesItAgain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	_, err := m.Deactivate(ctx, acme(env), fixtures.RuleVipOnlyID)
	require.NoError(t, err)

	r, err := m.Publish(ctx, acme(env), fixtures.RuleVipOnlyID)

	require.NoError(t, err)
	live, draft := rules.LiveAndDraft(r)
	assert.Equal(t, rule.StatusActive, r.Status)
	assert.EqualValues(t, fixtures.RuleVipOnlyID, live.ID)
	assert.Nil(t, draft)
}

func TestPublishRefusesADraftWithNoActions(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	none := []rules.Action{}
	_, err := m.Update(ctx, acme(env), fixtures.RuleTagClickedID, rules.UpdateInput{Actions: &none})
	require.NoError(t, err)

	_, err = m.Publish(ctx, acme(env), fixtures.RuleTagClickedID)

	require.ErrorIs(t, err, rules.ErrInvalid)
	r, err := m.Get(ctx, acme(env), fixtures.RuleTagClickedID)
	require.NoError(t, err)
	live, _ := rules.LiveAndDraft(r)
	assert.EqualValues(t, fixtures.RuleTagClickedLiveVersionID, live.ID)
}

func TestRevertDropsTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)

	r, err := m.Revert(ctx, acme(env), fixtures.RuleTagClickedID)

	require.NoError(t, err)
	live, draft := rules.LiveAndDraft(r)
	assert.Nil(t, draft)
	assert.EqualValues(t, fixtures.RuleTagClickedLiveVersionID, live.ID)
	gone, err := acme(env).RuleVersion().Query().Where(ruleversion.ID(fixtures.RuleTagClickedDraftVersionID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, gone)
}

func TestRevertRefusesARuleWithNoDraft(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := rules.New(env.Cipher, env.Bus).Revert(context.Background(), acme(env), fixtures.RuleVipOnlyID)

	assert.ErrorIs(t, err, rules.ErrNothingToRevert)
}

// A Rule never published has only its draft, which is its whole definition: nothing to go back to.
func TestRevertRefusesARuleNeverPublished(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := rules.New(env.Cipher, env.Bus).Revert(context.Background(), acme(env), fixtures.RuleInactiveID)

	assert.ErrorIs(t, err, rules.ErrNothingToRevert)
}

func TestRevertOfAnotherWorkspacesRuleIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := rules.New(env.Cipher, env.Bus).Revert(context.Background(), acme(env), fixtures.RuleGlobexID)

	assert.ErrorIs(t, err, rules.ErrNotFound)
}

// The edit after a Revert is the version above the live one again.
func TestAnEditAfterRevertMakesANewDraft(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	_, err := m.Revert(ctx, acme(env), fixtures.RuleTagClickedID)
	require.NoError(t, err)
	actions := tagAction("rule-clicked-again")

	r, err := m.Update(ctx, acme(env), fixtures.RuleTagClickedID, rules.UpdateInput{Actions: &actions})

	require.NoError(t, err)
	_, draft := rules.LiveAndDraft(r)
	require.NotNil(t, draft)
	assert.Equal(t, 2, draft.Version)
	assert.Equal(t, []string{"rule-clicked-again"}, appliedTags(t, draft))
}

// A draft names a Tag as much as a live version does (spec #767, "In use"): the Rule never
// published and the draft of an active Rule both hold theirs.
func TestADraftThatNamesATagUsesIt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	neverPublished, err := rules.UsingTag(ctx, acme(env), "rule-inactive")
	require.NoError(t, err)
	draftOfActive, err := rules.UsingTag(ctx, acme(env), "rule-clicked-v2")
	require.NoError(t, err)

	assert.Equal(t, []int64{fixtures.RuleInactiveID}, lo.Map(neverPublished, func(r *ent.Rule, _ int) int64 { return r.ID }))
	assert.Equal(t, []int64{fixtures.RuleTagClickedID}, lo.Map(draftOfActive, func(r *ent.Rule, _ int) int64 { return r.ID }))
}

func TestADraftThatEnrollsInAnAutomationKeepsItInUse(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := rules.New(env.Cipher, env.Bus)
	enroll := []rules.Action{{Type: rules.ActionEnroll, AutomationID: fixtures.AutomationWelcomeSeriesID}}
	_, err := m.Update(ctx, acme(env), fixtures.RuleInactiveID, rules.UpdateInput{Actions: &enroll})
	require.NoError(t, err)

	using, err := rules.EnrollingInto(ctx, acme(env), fixtures.AutomationWelcomeSeriesID)

	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.RuleInactiveID}, lo.Map(using, func(r *ent.Rule, _ int) int64 { return r.ID }))
}

// A dry run tries what the author is editing: the draft when there is one.
func TestDryRunUsesTheDraft(t *testing.T) {
	env := testhelper.Setup(t)

	res := dryRun(t, env, fixtures.RuleTagClickedID, fixtures.EventRuleClickLiamID)

	assert.Equal(t, rules.WouldFire, res.Outcome)
	require.Len(t, res.Actions, 1)
	assert.Contains(t, res.Actions[0].Summary, `"rule-clicked-v2"`)
}

// The first draft, a Publish and a Revert move the Rule's pointers, so each is an audited write of
// the Rule; the Actions' values never reach an entry.
func TestDraftPublishAndRevertAreAudited(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	actor := events.Actor{Kind: events.ActorUser, ID: strconv.Itoa(fixtures.OwnerJohnID), Name: "John"}
	s := env.Bus.Act(acme(env), actor)
	m := rules.New(env.Cipher, env.Bus)
	actions := tagAction("secretish")
	_, err := m.Update(ctx, s, fixtures.RuleVipOnlyID, rules.UpdateInput{Actions: &actions})
	require.NoError(t, err)

	_, err = m.Revert(ctx, s, fixtures.RuleVipOnlyID)
	require.NoError(t, err)

	entries := lo.FilterMap(env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) (*events.AuditEntry, bool) {
		e, ok := ev.(*events.AuditEntry)
		return e, ok && e.TargetType == "rule" && e.TargetID == strconv.Itoa(fixtures.RuleVipOnlyID)
	})
	diffs := lo.Map(entries, func(e *events.AuditEntry, _ int) map[string]any { return e.Diff })
	assert.Len(t, entries, 2, "the draft made and the draft reverted")
	assert.Contains(t, diffs[0], rule.FieldDraftVersionID)
	assert.Contains(t, diffs[1], rule.FieldDraftVersionID)
}
