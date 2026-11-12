package external_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// #748: a token with only the read and write scopes edits an active Rule's Trigger events,
// Condition and Actions. The edit is a draft, and the Rule keeps firing on its live version
// until a token with automations:publish publishes it (ADR 0065).
func TestAnAuthoringTokenCannotChangeWhatAnActiveRuleDoes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	author := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)
	id := entityIDString(fixtures.RuleVipOnlyID)

	res, err := author.RulesUpdate(ctx, &externalapi.UpdateRuleInput{Revision: externalapi.Revision(env.RuleRevision(ctx, t, int64(id))),
		TriggerEvents: []string{"rule_vip", "anything"},
		Condition:     externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[]}`)),
		Actions:       []externalapi.RuleAction{{Type: externalapi.RuleActionTypeApplyTag, Tag: externalapi.NewOptString("hijacked")}},
	}, externalapi.RulesUpdateParams{ID: id})
	require.NoError(t, err)
	live, _ := res.Live.Get()
	draft, drafted := res.Draft.Get()
	require.True(t, drafted)
	assert.Equal(t, []string{"rule_vip"}, live.TriggerEvents)
	assert.Equal(t, "rule-vip-seen", live.Actions[0].Tag.Or(""))
	assert.Equal(t, []string{"rule_vip", "anything"}, draft.TriggerEvents)
	assert.Equal(t, "hijacked", draft.Actions[0].Tag.Or(""))

	m := rules.New(env.Cipher, env.Bus, nil, time.Now)
	s := env.DB.Scoped(fixtures.AcmeID)
	bob, err := m.Evaluate(ctx, s, rules.Event{ContactID: fixtures.ContactBobID, Action: "rule_vip", SourceID: "evt-748-bob", RecordedAt: time.Now()})
	require.NoError(t, err)
	assert.Empty(t, bob, "the live Condition still needs the vip Tag")
	alice, err := m.Evaluate(ctx, s, rules.Event{ContactID: fixtures.ContactAliceID, Action: "rule_vip", SourceID: "evt-748-alice", RecordedAt: time.Now()})
	require.NoError(t, err)
	require.Len(t, alice, 1)
	runs, err := rules.FiringActions(ctx, s, alice[0])
	require.NoError(t, err)
	assert.Equal(t, "rule-vip-seen", runs[0].Tag, "the Firing runs the live Actions")

	_, err = author.RulesPublish(ctx, &externalapi.RevisionInput{Revision: externalapi.Revision(env.RuleRevision(ctx, t, int64(id)))}, externalapi.RulesPublishParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	publisher := env.ExternalScoped(t, apitokens.ScopeAutomationsPublish)
	now, err := publisher.RulesPublish(ctx, &externalapi.RevisionInput{Revision: externalapi.Revision(env.RuleRevision(ctx, t, int64(id)))}, externalapi.RulesPublishParams{ID: id})
	require.NoError(t, err)
	newLive, _ := now.Live.Get()
	assert.EqualValues(t, 2, newLive.Number)
	assert.Equal(t, "hijacked", newLive.Actions[0].Tag.Or(""))
	assert.True(t, now.Draft.Null)
}

// Revert is authoring: it drops the draft with the write scope, and refuses a Rule with nothing
// to revert.
func TestExternalRevertDropsTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	id := entityIDString(fixtures.RuleTagClickedID)

	reader := env.ExternalScoped(t, apitokens.ScopeAutomationsRead)
	_, err := reader.RulesRevert(ctx, &externalapi.RevisionInput{Revision: externalapi.Revision(env.RuleRevision(ctx, t, int64(id)))}, externalapi.RulesRevertParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	author := env.ExternalScoped(t, apitokens.ScopeAutomationsWrite)
	res, err := author.RulesRevert(ctx, &externalapi.RevisionInput{Revision: externalapi.Revision(env.RuleRevision(ctx, t, int64(id)))}, externalapi.RulesRevertParams{ID: id})
	require.NoError(t, err)
	assert.True(t, res.Draft.Null)
	live, _ := res.Live.Get()
	assert.Equal(t, "rule-clicked", live.Actions[0].Tag.Or(""))

	_, err = author.RulesRevert(ctx, &externalapi.RevisionInput{Revision: externalapi.Revision(env.RuleRevision(ctx, t, int64(id)))}, externalapi.RulesRevertParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
}
