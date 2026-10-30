package site_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationversion"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// auditedStatuses returns the status changes the Audit log recorded for one target, in order.
func auditedStatuses(t *testing.T, env *testhelper.TestEnv, targetType string, id int64) []any {
	t.Helper()
	entries := lo.FilterMap(env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) (*events.AuditEntry, bool) {
		e, ok := ev.(*events.AuditEntry)
		return e, ok && e.TargetType == targetType && e.TargetID == strconv.FormatInt(id, 10)
	})
	return lo.Map(entries, func(e *events.AuditEntry, _ int) any { return []any{e.Action, e.Diff["status"]} })
}

// Publish and Deactivate of an Automation are Audited as the status change they make (ADR 0022).
func TestSiteAutomationPublishAndDeactivateAreAudited(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	params := siteapi.SiteAutomationsPublishParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AutomationWinBackID)}

	_, err := c.SiteAutomationsPublish(ctx, params)
	require.NoError(t, err)
	_, err = c.SiteAutomationsDeactivate(ctx, siteapi.SiteAutomationsDeactivateParams(params))
	require.NoError(t, err)

	got := auditedStatuses(t, env, "automation", fixtures.AutomationWinBackID)
	assert.Equal(t, []any{
		[]any{"automation.update", events.Change("inactive", "active")},
		[]any{"automation.update", events.Change("active", "inactive")},
	}, got)
}

// auditedVersionMoves returns, per Audit entry of the Automation, its action and how its live and
// draft pointers changed, in order.
func auditedVersionMoves(t *testing.T, env *testhelper.TestEnv, id int64) []any {
	t.Helper()
	entries := lo.FilterMap(env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) (*events.AuditEntry, bool) {
		e, ok := ev.(*events.AuditEntry)
		return e, ok && e.TargetType == "automation" && e.TargetID == strconv.FormatInt(id, 10)
	})
	return lo.Map(entries, func(e *events.AuditEntry, _ int) any {
		return []any{e.Action, pointerMove(e.Diff[automation.FieldLiveVersionID]), pointerMove(e.Diff[automation.FieldDraftVersionID])}
	})
}

// pointerMove writes a Diff entry of a version pointer as "from->to" ("" when it did not change),
// whatever number type the entry was decoded into.
func pointerMove(entry any) string {
	m, ok := entry.(map[string]any)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%v->%v", m[events.DiffFrom], m[events.DiffTo])
}

// Publish and Revert are Audited as the move of the live and draft pointers they make (ADR 0022,
// ADR 0065): Publish makes the draft live, Revert drops it.
func TestSiteAutomationPublishAndRevertAreAudited(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	params := siteapi.SiteAutomationsRevertParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AutomationWelcomeSeriesID)}

	_, err := c.SiteAutomationsRevert(ctx, params)
	require.NoError(t, err)
	edited, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{TriggerEvent: siteapi.NewOptString("signup_edited")},
		siteapi.SiteAutomationsUpdateParams(params))
	require.NoError(t, err)
	draft := edited.(*siteapi.SiteAutomationResource).Draft.Value
	_, err = c.SiteAutomationsPublish(ctx, siteapi.SiteAutomationsPublishParams(params))
	require.NoError(t, err)

	got := auditedVersionMoves(t, env, fixtures.AutomationWelcomeSeriesID)
	newID := versionIDOf(t, env, fixtures.AutomationWelcomeSeriesID, int(draft.Number))
	assert.Equal(t, []any{
		[]any{"automation.update", "", fmt.Sprintf("%d-><nil>", fixtures.AutomationWelcomeSeriesDraftVersionID)},
		[]any{"automation.update", "", fmt.Sprintf("<nil>->%d", newID)},
		[]any{"automation.update", fmt.Sprintf("%d->%d", fixtures.AutomationWelcomeSeriesVersionID, newID), fmt.Sprintf("%d-><nil>", newID)},
	}, got, "Revert, the next draft, Publish")
}

// versionIDOf is the id of the Automation's version with that number.
func versionIDOf(t *testing.T, env *testhelper.TestEnv, automationID int64, number int) int64 {
	t.Helper()
	id, err := env.DB.AutomationVersion.Query().
		Where(automationversion.AutomationID(automationID), automationversion.Version(number)).
		OnlyID(context.Background())
	require.NoError(t, err)
	return id
}

// Publish and Deactivate of a Rule are Audited as the status change they make (ADR 0022).
func TestSiteRulePublishAndDeactivateAreAudited(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	params := siteapi.SiteRulesPublishParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.RuleInactiveID)}

	_, err := c.SiteRulesPublish(ctx, params)
	require.NoError(t, err)
	_, err = c.SiteRulesDeactivate(ctx, siteapi.SiteRulesDeactivateParams(params))
	require.NoError(t, err)

	got := auditedStatuses(t, env, "rule", fixtures.RuleInactiveID)
	assert.Equal(t, []any{
		[]any{"rule.update", events.Change("inactive", "active")},
		[]any{"rule.update", events.Change("active", "inactive")},
	}, got)
}
