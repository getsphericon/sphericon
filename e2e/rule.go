//go:build e2e

package e2e

import (
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
)

// Rule is what a scenario chooses about a Rule: one Trigger event and one apply-Tag Action.
type Rule struct {
	TriggerEvent string
	// Tag is applied to the Contact the Event is for.
	Tag string
}

// ActivateRule creates a Rule that applies Tag when TriggerEvent is recorded, and activates it.
// Creation always stores a draft; activating is a separate step.
func (w *Workspace) ActivateRule(r Rule) {
	w.t.Helper()
	ctx := w.t.Context()
	res, err := w.api.RulesCreate(ctx, &externalapi.CreateRuleInput{
		Name:          "e2e rule " + uniq(),
		TriggerEvents: []string{r.TriggerEvent},
		Actions:       []externalapi.RuleAction{{Type: externalapi.RuleActionTypeApplyTag, Tag: externalapi.NewOptString(r.Tag)}},
	})
	created := ok[externalapi.RuleResource](w.t, "create rule", res, err)

	act, err := w.api.RulesActivate(ctx, externalapi.RulesActivateParams{ID: created.ID})
	got := ok[externalapi.RuleResource](w.t, "activate rule", act, err)
	require.Equal(w.t, externalapi.RuleStatusActive, got.Status)
}

// Visit is what the tracking snippet does on a customer's site: it identifies the visitor as the
// Contact with this email, then records the action for that visitor, through /collect.
func (w *Workspace) Visit(email, action string) {
	w.t.Helper()
	ctx := w.t.Context()
	visitor := "visitor-" + uniq()
	identified, err := w.collect.CollectIdentifyCreate(ctx, &collectapi.CollectIdentifyInput{
		VisitorId: visitor,
		Email:     collectapi.NewOptNilEmailAddress(collectapi.EmailAddress(email)),
	})
	require.NoError(w.t, err, "identify visitor")
	require.IsType(w.t, &collectapi.CollectOkResponse{}, identified, "identify visitor")

	recorded, err := w.collect.CollectEventsCreate(ctx, &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{{VisitorId: visitor, Action: action}},
	})
	require.NoError(w.t, err, "collect event")
	require.IsType(w.t, &collectapi.CollectEventsCreateNoContent{}, recorded, "collect event")
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
