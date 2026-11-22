//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// Rule is what a scenario chooses about a Rule: one Trigger event, one apply-Tag Action and,
// optionally, a second Action that enrolls the Contact in an Automation.
type Rule struct {
	TriggerEvent string
	// Tag is applied to the Contact the Event is for.
	Tag string
	// EnrollIn is the id of an Automation (from PublishAutomation) the Contact is enrolled in;
	// zero adds no enroll Action.
	EnrollIn int64
	// Condition is the typed tree of a Rule (ADR 0181), written as JSON; empty sends none, and the
	// Rule fires for every Contact.
	Condition string
}

// PublishedRule is a Rule the console published, as a later step edits it.
type PublishedRule struct {
	ID   siteapi.EntityId
	Rule Rule
}

// PublishRule creates a Rule through the site API (the external API offers no enroll Action)
// and publishes it. Creation always stores an inactive Rule; publishing is a separate step.
func (w *Workspace) PublishRule(r Rule) PublishedRule {
	w.t.Helper()
	ctx := w.t.Context()
	ws := siteapi.WorkspaceId(w.publicID)
	res, err := w.site.SiteRulesCreate(ctx, &siteapi.CreateRuleInput{
		Name:          siteapi.Name("e2e rule " + uniq()),
		TriggerEvents: []string{r.TriggerEvent},
		Condition:     w.ruleCondition(r.Condition),
		Actions:       ruleActions(r),
	}, siteapi.SiteRulesCreateParams{WorkspaceId: ws})
	created := ok[siteapi.SiteRuleResource](w.t, "create rule", res, err)
	w.requireRuleCondition(r.Condition, created.Draft)

	w.publishRule(created)
	return PublishedRule{ID: created.ID, Rule: r}
}

// EditRuleCondition is a person changing a published Rule's Condition in the console: the edit
// is a draft, on the revision they read, and they publish it.
func (w *Workspace) EditRuleCondition(p PublishedRule, condition string) {
	w.t.Helper()
	ctx := w.t.Context()
	ws := siteapi.WorkspaceId(w.publicID)
	read, err := w.site.SiteRulesGet(ctx, siteapi.SiteRulesGetParams{WorkspaceId: ws, ID: p.ID})
	current := ok[siteapi.SiteRuleResource](w.t, "read rule", read, err)
	res, err := w.site.SiteRulesUpdate(ctx, &siteapi.UpdateRuleInput{
		Revision:      current.Revision,
		TriggerEvents: []string{p.Rule.TriggerEvent},
		Condition:     w.ruleCondition(condition),
		Actions:       ruleActions(p.Rule),
	}, siteapi.SiteRulesUpdateParams{WorkspaceId: ws, ID: p.ID})
	edited := ok[siteapi.SiteRuleResource](w.t, "edit rule", res, err)
	w.requireRuleCondition(condition, edited.Draft)

	w.publishRule(edited)
}

// publishRule publishes the Rule's draft and checks that the Rule is active.
func (w *Workspace) publishRule(r *siteapi.SiteRuleResource) {
	w.t.Helper()
	params := siteapi.SiteRulesPublishParams{WorkspaceId: siteapi.WorkspaceId(w.publicID), ID: r.ID}
	published, err := w.site.SiteRulesPublish(w.t.Context(), &siteapi.RevisionInput{Revision: r.Revision}, params)
	got := ok[siteapi.SiteRuleResource](w.t, "publish rule", published, err)
	require.Equal(w.t, siteapi.RuleStatusActive, got.Status)
}

// ruleActions are the Rule's Actions: apply its Tag, then enroll the Contact if it names an
// Automation.
func ruleActions(r Rule) []siteapi.RuleActionInput {
	actions := []siteapi.RuleActionInput{{Type: siteapi.RuleActionTypeApplyTag, Tag: siteapi.NewOptString(r.Tag)}}
	if r.EnrollIn != 0 {
		actions = append(actions, siteapi.RuleActionInput{
			Type:         siteapi.RuleActionTypeEnrollAutomation,
			AutomationId: siteapi.NewOptEntityId(siteapi.EntityId(r.EnrollIn)),
		})
	}
	return actions
}

// ruleCondition is the Rule's Condition as the console sends it, none when it is empty.
func (w *Workspace) ruleCondition(condition string) siteapi.OptRuleConditionInput {
	w.t.Helper()
	if condition == "" {
		return siteapi.OptRuleConditionInput{}
	}
	return siteapi.NewOptRuleConditionInput(conditionOf[siteapi.RuleConditionInput](w.t, condition))
}

// requireRuleCondition checks that the draft a write left holds the Condition it sent.
func (w *Workspace) requireRuleCondition(condition string, draft siteapi.NilRuleVersion) {
	w.t.Helper()
	if condition == "" {
		return
	}
	require.False(w.t, draft.Null, "the write leaves a draft")
	requireCondition(w.t, condition, &draft.Value.Condition.Value)
}

// Visit is what the tracking snippet does on a customer's site: it identifies the visitor as the
// Contact with this email, then records the action for that visitor, through /collect.
func (w *Workspace) Visit(email, action string) {
	w.t.Helper()
	w.visit(email, action, collectapi.OptNilJsonObject{})
}

// VisitWith is Visit of an action that carries properties, a JSON object.
func (w *Workspace) VisitWith(email, action, properties string) {
	w.t.Helper()
	var props collectapi.JsonObject
	require.NoError(w.t, json.Unmarshal([]byte(properties), &props), "the properties are a JSON object")
	w.visit(email, action, collectapi.NewOptNilJsonObject(props))
}

// visit identifies the visitor and records the action with its properties, if any.
func (w *Workspace) visit(email, action string, properties collectapi.OptNilJsonObject) {
	w.t.Helper()
	ctx := w.t.Context()
	visitor := "visitor-" + uniq()
	_, err := w.collect.CollectIdentifyCreate(ctx, &collectapi.CollectIdentifyInput{
		VisitorId: visitor,
		Email:     collectapi.NewOptNilEmailAddress(collectapi.EmailAddress(email)),
	})
	require.NoError(w.t, err, "identify visitor")

	err = w.collect.CollectEventsCreate(ctx, &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{{VisitorId: visitor, Action: action, Properties: properties}},
	})
	require.NoError(w.t, err, "collect event")
}

// RequireTagged polls, through the Contact's data export, until the Contact with this email has
// the Tag. A Rule acts on a queue after the Event is recorded, so the Tag arrives asynchronously.
func (w *Workspace) RequireTagged(email, tag string) {
	w.t.Helper()
	require.Eventually(w.t, func() bool {
		doc, found := w.contactExport(email)
		return found && lo.ContainsBy(doc.Tags, func(t externalapi.ContactExportTag) bool { return strings.EqualFold(t.Name, tag) })
	}, EmailTimeout, 100*time.Millisecond, "%s was never tagged %q", email, tag)
}

// RequireNotTagged asserts the Contact has no such Tag right now. Call it after RequireTagged has
// observed the Rule's effect on another Contact, so the Event that did not match has been through
// the same pipeline.
func (w *Workspace) RequireNotTagged(email, tag string) {
	w.t.Helper()
	doc, found := w.contactExport(email)
	require.True(w.t, found, "export of %s", email)
	names := lo.Map(doc.Tags, func(t externalapi.ContactExportTag, _ int) string { return t.Name })
	require.NotContains(w.t, names, tag, "%s must not carry the Tag", email)
}
