package external_test

import (
	"testing"
	"time"

	"github.com/go-faster/jx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Conditions of the Segment "Pro & team members" (SegmentProPlan) before and after an edit.
const (
	proPlanCondition  = `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:plan","operator":"eq","value":"pro"}]}`
	freePlanCondition = `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:plan","operator":"eq","value":"free"}]}`
)

var audienceScopes = []string{
	"broadcasts:read", "broadcasts:write", "broadcasts:send", "segments:write", "contacts:write",
}

// recipientEmails is the addresses of the Broadcast's Recipients.
func recipientEmails(t *testing.T, env *testhelper.TestEnv, broadcastID int64) []string {
	t.Helper()
	rows, err := env.DB.BroadcastRecipient.Query().
		Where(broadcastrecipient.BroadcastID(broadcastID)).
		WithContact().
		All(t.Context())
	require.NoError(t, err)
	return lo.Map(rows, func(r *ent.BroadcastRecipient, _ int) string { return lo.FromPtr(r.Edges.Contact.Email) })
}

func scheduleBroadcast(t *testing.T, c *externalapi.Client, id externalapi.EntityId) {
	t.Helper()
	res, err := c.BroadcastsSchedule(t.Context(), &externalapi.ScheduleBroadcastInput{ScheduledAt: externalapi.Timestamp(time.Now().Add(time.Hour))},
		externalapi.BroadcastsScheduleParams{ID: id})
	require.NoError(t, err)
	require.IsTypef(t, &externalapi.BroadcastResource{}, res, "got %T", res)
}

func setSegmentCondition(t *testing.T, c *externalapi.Client, id externalapi.EntityId, def string) {
	t.Helper()
	res, err := c.SegmentsUpdate(t.Context(), &externalapi.UpdateSegmentInput{Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t, def))},
		externalapi.SegmentsUpdateParams{ID: id})
	require.NoError(t, err)
	require.IsTypef(t, &externalapi.SegmentResource{}, res, "got %T", res)
}

// createProContact records a Contact on the pro plan.
func createProContact(t *testing.T, c *externalapi.Client, email string) {
	t.Helper()
	res, err := c.ContactsCreate(t.Context(), &externalapi.CreateContactInput{
		Email:        externalapi.NewOptNilEmailAddress(externalapi.EmailAddress(email)),
		CustomFields: externalapi.NewOptNilJsonObject(externalapi.JsonObject{"plan": jx.Raw(`"pro"`)}),
	})
	require.NoError(t, err)
	require.IsTypef(t, &externalapi.ContactResource{}, res, "got %T", res)
}

// A scheduled Broadcast keeps the Condition its Segment had when it was scheduled (ADR 0065): a
// token that can edit the Segment cannot change who receives it, while a Contact who comes to
// match the copy after scheduling still does.
func TestExternalScheduledBroadcastSendsToTheConditionCopiedAtSchedule(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, audienceScopes...)
	id := entityIDString(fixtures.BroadcastProSegmentID)

	scheduleBroadcast(t, c, id)
	setSegmentCondition(t, c, proSegment, freePlanCondition)
	createProContact(t, c, "late.pro@example.com")
	env.DeliverQueuedBroadcasts(t)

	got := recipientEmails(t, env, fixtures.BroadcastProSegmentID)
	assert.Contains(t, got, "late.pro@example.com", "a Contact who matched after scheduling receives it")
	assert.Contains(t, got, fixtures.ContactLiamEmail, "a pro Contact receives it")
	assert.NotContains(t, got, fixtures.ContactNoahEmail, "a free Contact, who matches only the edited Segment, does not")
}

// A Tag named only in a scheduled Broadcast's copied Condition is in use by that Broadcast, while
// its Segment of origin is free to archive (ADR 0065).
func TestExternalTagNamedInAScheduledBroadcastsCopyIsInUse(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write", "segments:write")

	res, err := c.TagsDelete(t.Context(), externalapi.TagsDeleteParams{ID: entityIDString(fixtures.TagRenewalDueID)})
	require.NoError(t, err)
	refused, ok := res.(*externalapi.InUseProblemDetails)
	require.Truef(t, ok, "got %T", res)
	users := lo.Map(refused.UsedBy, func(u externalapi.UsedBy, _ int) string { return string(u.Kind) + ":" + u.Name })
	assert.Equal(t, []string{string(externalapi.UsedByKindBroadcast) + ":" + fixtures.BroadcastScheduledRenewalName}, users)

	archived, err := c.SegmentsArchive(t.Context(), externalapi.SegmentsArchiveParams{ID: entityIDString(fixtures.SegmentRenewalAudienceID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentResource{}, archived)
}

// An archived Segment takes no part in new work (ADR 0049): a Broadcast unscheduled after its
// Segment was archived cannot be scheduled again.
func TestExternalBroadcastOfAnArchivedSegmentIsNotScheduledAgain(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, audienceScopes...)
	id := entityIDString(fixtures.BroadcastScheduledRenewalID)

	archived, err := c.SegmentsArchive(t.Context(), externalapi.SegmentsArchiveParams{ID: entityIDString(fixtures.SegmentRenewalAudienceID)})
	require.NoError(t, err)
	require.IsType(t, &externalapi.SegmentResource{}, archived)
	back, err := c.BroadcastsUnschedule(t.Context(), externalapi.BroadcastsUnscheduleParams{ID: id})
	require.NoError(t, err)
	require.IsType(t, &externalapi.BroadcastResource{}, back)

	res, err := c.BroadcastsSchedule(t.Context(), &externalapi.ScheduleBroadcastInput{ScheduledAt: externalapi.Timestamp(time.Now().Add(time.Hour))},
		externalapi.BroadcastsScheduleParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsScheduleUnprocessableEntity{}, res)
}

// Unscheduling drops the copy, and scheduling again takes the Segment's current Condition.
func TestExternalRescheduledBroadcastTakesTheSegmentsCurrentCondition(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, audienceScopes...)
	id := entityIDString(fixtures.BroadcastProSegmentID)

	scheduleBroadcast(t, c, id)
	setSegmentCondition(t, c, proSegment, freePlanCondition)
	res, err := c.BroadcastsUnschedule(t.Context(), externalapi.BroadcastsUnscheduleParams{ID: id})
	require.NoError(t, err)
	require.IsTypef(t, &externalapi.BroadcastResource{}, res, "got %T", res)
	scheduleBroadcast(t, c, id)
	setSegmentCondition(t, c, proSegment, proPlanCondition)
	env.DeliverQueuedBroadcasts(t)

	got := recipientEmails(t, env, fixtures.BroadcastProSegmentID)
	assert.Contains(t, got, fixtures.ContactNoahEmail, "a free Contact matches the Condition copied at the second schedule")
	assert.NotContains(t, got, fixtures.ContactLiamEmail, "a pro Contact matches only the Segment's later edit")
}
