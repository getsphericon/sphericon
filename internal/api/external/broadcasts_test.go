package external_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Fixtures (workspace acme): BroadcastScheduled and BroadcastFailed (2 skipped) by id;
// SegmentProPlan is the rule segment "Pro & team members".
var (
	schedBroadcast  = entityIDString(fixtures.BroadcastScheduledID)
	failedBroadcast = entityIDString(fixtures.BroadcastFailedID)
	proSegment      = entityIDString(fixtures.SegmentProPlanID)
)

var (
	draftBroadcast = entityIDString(fixtures.BroadcastDraftID)
	sentBroadcast  = entityIDString(fixtures.BroadcastSentID)
)

var authorScopes = []string{"broadcasts:read", "broadcasts:write"}

func TestExternalBroadcastsAreCreatedAsDraftsAndEditedWhileDraft(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()

	created, err := c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
		Name:    "Spring sale",
		Subject: externalapi.NewOptString("Big news"),
	})
	require.NoError(t, err)
	b, ok := created.(*externalapi.BroadcastResource)
	require.Truef(t, ok, "got %T", created)
	assert.Equal(t, externalapi.BroadcastStatusDraft, b.Status)
	assert.Equal(t, "Big news", b.Subject)
	assert.False(t, b.SegmentId.IsSet(), "no Segment yet")

	got, err := c.BroadcastsGet(ctx, externalapi.BroadcastsGetParams{ID: b.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastResource{}, got)

	updated, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Name: externalapi.NewOptString("Spring sale v2")},
		externalapi.BroadcastsUpdateParams{ID: b.ID})
	require.NoError(t, err)
	assert.Equal(t, "Spring sale v2", updated.(*externalapi.BroadcastResource).Name)

	// A scheduled broadcast is no longer editable.
	locked, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Name: externalapi.NewOptString("x")},
		externalapi.BroadcastsUpdateParams{ID: schedBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsUpdateUnprocessableEntity{}, locked)

	list, err := c.BroadcastsList(ctx, externalapi.BroadcastsListParams{})
	require.NoError(t, err)
	listed, ok := list.(*externalapi.BroadcastsListOK)
	require.Truef(t, ok, "got %T", list)
	assert.NotEmpty(t, listed.Items)
}

// The one pagination smoke of the external surface: the envelope is copied from the module's page.
// The /api body of an update is a merge patch with the fields /site has: a null clears the
// Segment and the From name, an absent key keeps them.
func TestExternalBroadcastsUpdateClearsANullAndKeepsAnAbsentField(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()
	params := externalapi.BroadcastsUpdateParams{ID: draftBroadcast}

	_, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{
		SegmentId: externalapi.NewOptNilEntityId(proSegment), FromName: externalapi.NewOptNilString("Acme"),
	}, params)
	require.NoError(t, err)

	var none externalapi.OptNilString
	none.SetToNull()
	res, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{FromName: none}, params)
	require.NoError(t, err)
	got, ok := res.(*externalapi.BroadcastResource)
	require.Truef(t, ok, "got %T", res)
	assert.Empty(t, got.FromName.Or(""), "a null clears the From name")
	assert.Equal(t, externalapi.NewOptNilEntityId(proSegment), got.SegmentId, "an absent Segment is kept")
}

func TestExternalBroadcastsListPaginates(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)

	res, err := c.BroadcastsList(context.Background(), externalapi.BroadcastsListParams{
		Page: externalapi.NewOptInt32(2), PageSize: externalapi.NewOptInt32(2),
	})
	require.NoError(t, err)
	page, ok := res.(*externalapi.BroadcastsListOK)
	require.Truef(t, ok, "got %T", res)
	assert.Len(t, page.Items, 2)
	assert.EqualValues(t, 2, page.Page)
	assert.EqualValues(t, 2, page.PageSize)
	assert.Greater(t, page.TotalItems, int32(4))
	assert.Greater(t, page.TotalPages, int32(2))
}

func TestExternalBroadcastsDeleteOnlyDrafts(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()

	out, err := c.BroadcastsDelete(ctx, externalapi.BroadcastsDeleteParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsDeleteNoContent{}, out)

	out, err = c.BroadcastsDelete(ctx, externalapi.BroadcastsDeleteParams{ID: sentBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsDeleteUnprocessableEntity{}, out, "a sent broadcast is history")
}

func TestExternalBroadcastSegmentIsASegmentOrEveryone(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()

	set, err := c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{SegmentId: externalapi.NewNilEntityId(proSegment)},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	require.NoError(t, err)
	b := set.(*externalapi.BroadcastResource)
	id, ok := b.SegmentId.Get()
	require.True(t, ok)
	assert.Equal(t, proSegment, id)
	assert.Equal(t, externalapi.BroadcastStatusDraft, b.Status, "setting a Segment never sends or schedules")

	cleared, err := c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{SegmentId: externalapi.NilEntityId{Null: true}},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.False(t, cleared.(*externalapi.BroadcastResource).SegmentId.IsSet())

	unknown, err := c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{SegmentId: externalapi.NewNilEntityId(999999)},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsSetSegmentUnprocessableEntity{}, unknown)

	locked, err := c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{SegmentId: externalapi.NewNilEntityId(proSegment)},
		externalapi.BroadcastsSetSegmentParams{ID: schedBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsSetSegmentUnprocessableEntity{}, locked)
}

func TestExternalBroadcastTestSendGoesToOneAddressOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()

	out, err := c.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Email: "qa@test.dev"},
		externalapi.BroadcastsTestSendParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsTestSendNoContent{}, out)

	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "qa@test.dev", msgs[0].To)
	assert.Equal(t, broadcast.StatusDraft, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status, "a test send does not touch the lifecycle")
}

func TestExternalBroadcastReport(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()

	got, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: failedBroadcast})
	require.NoError(t, err)
	r, ok := got.(*externalapi.BroadcastReport)
	require.Truef(t, ok, "got %T", got)
	assert.Equal(t, externalapi.BroadcastStatusFailed, r.Status)
	assert.EqualValues(t, 2, r.SkippedCount)
	assert.EqualValues(t, 3, r.FailedCount)

	sent, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: sentBroadcast})
	require.NoError(t, err)
	sr := sent.(*externalapi.BroadcastReport)
	// The figures are the recipients' (fixtures: 11 sent, 5 opened, 2 clicked).
	assert.EqualValues(t, 11, sr.RecipientsTotal)
	assert.EqualValues(t, 11, sr.SentCount)
	assert.EqualValues(t, 5, sr.OpenedCount)
	assert.EqualValues(t, 2, sr.ClickedCount)
}

func TestExternalBroadcastsRequireScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	readOnly := env.ExternalScoped(t, "broadcasts:read")
	noScope := env.ExternalScoped(t, "contacts:read")

	// read scope reads...
	got, err := readOnly.BroadcastsGet(ctx, externalapi.BroadcastsGetParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastResource{}, got)
	rep, err := readOnly.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: sentBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastReport{}, rep)

	// ...but cannot write, set the Segment or test-send.
	created, err := readOnly.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{Name: "x"})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsCreateForbidden{}, created)
	upd, err := readOnly.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{}, externalapi.BroadcastsUpdateParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsUpdateForbidden{}, upd)
	del, err := readOnly.BroadcastsDelete(ctx, externalapi.BroadcastsDeleteParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsDeleteForbidden{}, del)
	aud, err := readOnly.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{SegmentId: externalapi.NilEntityId{Null: true}},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsSetSegmentForbidden{}, aud)
	ts, err := readOnly.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Email: "qa@test.dev"}, externalapi.BroadcastsTestSendParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsTestSendForbidden{}, ts)
	assert.Empty(t, env.CustomerMail.Messages())

	// A token without any broadcast scope reads nothing.
	list, err := noScope.BroadcastsList(ctx, externalapi.BroadcastsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsListForbidden{}, list)
	rep, err = noScope.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: sentBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsReportForbidden{}, rep)
}

func TestExternalBroadcastScheduleNeedsTheSendScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	when := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	body := &externalapi.ScheduleBroadcastInput{ScheduledAt: externalapi.Timestamp(when)}

	// Authoring scopes are not enough to send, in either direction.
	author := env.ExternalScoped(t, authorScopes...)
	res, err := author.BroadcastsSchedule(ctx, body, externalapi.BroadcastsScheduleParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsScheduleForbidden{}, res)
	un, err := author.BroadcastsUnschedule(ctx, externalapi.BroadcastsUnscheduleParams{ID: schedBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsUnscheduleForbidden{}, un)
	assert.Equal(t, broadcast.StatusDraft, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status)
	assert.Equal(t, broadcast.StatusScheduled, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastScheduledID).Status)

	// emails:send is a different lock.
	emailer := env.ExternalScoped(t, "emails:send")
	res, err = emailer.BroadcastsSchedule(ctx, body, externalapi.BroadcastsScheduleParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsScheduleForbidden{}, res)

	// broadcasts:send alone schedules and unschedules.
	sender := env.ExternalScoped(t, "broadcasts:send")
	res, err = sender.BroadcastsSchedule(ctx, body, externalapi.BroadcastsScheduleParams{ID: draftBroadcast})
	require.NoError(t, err)
	b, ok := res.(*externalapi.BroadcastResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, externalapi.BroadcastStatusScheduled, b.Status)
	sched, ok := b.ScheduledAt.Get()
	require.True(t, ok)
	assert.True(t, when.Equal(time.Time(sched)))

	un, err = sender.BroadcastsUnschedule(ctx, externalapi.BroadcastsUnscheduleParams{ID: draftBroadcast})
	require.NoError(t, err)
	b, ok = un.(*externalapi.BroadcastResource)
	require.Truef(t, ok, "got %T", un)
	assert.Equal(t, externalapi.BroadcastStatusDraft, b.Status)
	assert.Nil(t, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).ScheduledAt)
}

func TestExternalBroadcastScheduleRefusesWhatCannotBeScheduled(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "broadcasts:send")
	body := &externalapi.ScheduleBroadcastInput{ScheduledAt: externalapi.Timestamp(time.Now().Add(time.Hour))}

	res, err := c.BroadcastsSchedule(ctx, body, externalapi.BroadcastsScheduleParams{ID: sentBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsScheduleUnprocessableEntity{}, res, "a sent broadcast is history")
	res, err = c.BroadcastsSchedule(ctx, body, externalapi.BroadcastsScheduleParams{ID: 999999})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsScheduleNotFound{}, res)

	un, err := c.BroadcastsUnschedule(ctx, externalapi.BroadcastsUnscheduleParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsUnscheduleUnprocessableEntity{}, un, "nothing to unschedule on a draft")
	un, err = c.BroadcastsUnschedule(ctx, externalapi.BroadcastsUnscheduleParams{ID: 999999})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsUnscheduleNotFound{}, un)
}

func TestExternalBroadcastsAreIsolatedToTheTokensWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// The Globex tenant's committed broadcast and segment.
	foreignID := externalapi.EntityId(fixtures.BroadcastGlobexID)
	foreignSegID := int64(fixtures.SegmentGlobexID)

	c := env.ExternalScoped(t, authorScopes...) // bound to workspace 1

	list, err := c.BroadcastsList(ctx, externalapi.BroadcastsListParams{})
	require.NoError(t, err)
	for _, item := range list.(*externalapi.BroadcastsListOK).Items {
		assert.NotEqual(t, foreignID, item.ID, "another tenant's broadcast leaked")
	}

	get, err := c.BroadcastsGet(ctx, externalapi.BroadcastsGetParams{ID: foreignID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsGetNotFound{}, get)
	upd, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Name: externalapi.NewOptString("hijack")}, externalapi.BroadcastsUpdateParams{ID: foreignID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsUpdateNotFound{}, upd)
	del, err := c.BroadcastsDelete(ctx, externalapi.BroadcastsDeleteParams{ID: foreignID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsDeleteNotFound{}, del)
	rep, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: foreignID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsReportNotFound{}, rep)
	ts, err := c.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Email: "qa@test.dev"}, externalapi.BroadcastsTestSendParams{ID: foreignID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsTestSendNotFound{}, ts)
	assert.Empty(t, env.CustomerMail.Messages())
	aud, err := c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{SegmentId: externalapi.NilEntityId{Null: true}}, externalapi.BroadcastsSetSegmentParams{ID: foreignID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsSetSegmentNotFound{}, aud)

	// Another workspace's segment cannot be the Segment of our broadcast.
	cross, err := c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{SegmentId: externalapi.NewNilEntityId(externalapi.EntityId(foreignSegID))},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsSetSegmentUnprocessableEntity{}, cross)

	assert.Equal(t, fixtures.BroadcastGlobexName, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastGlobexID).Name, "the foreign broadcast is untouched")
}

// An archived Segment leaves new work: a draft cannot be pointed at it, and a refused call
// changes nothing.
func TestExternalBroadcastSegmentRefusesAnArchivedSegment(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()
	before := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)

	refused, err := c.BroadcastsSetSegment(ctx,
		&externalapi.SetBroadcastSegmentInput{SegmentId: externalapi.NewNilEntityId(entityIDString(fixtures.SegmentArchivedID))},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsSetSegmentUnprocessableEntity{}, refused)

	after := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, before.SegmentID, after.SegmentID)
}

// A Broadcast that already points at an archived Segment keeps showing it.
func TestExternalBroadcastKeepsShowingItsArchivedSegment(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "broadcasts:read")
	ctx := context.Background()

	got, err := c.BroadcastsGet(ctx, externalapi.BroadcastsGetParams{ID: entityIDString(fixtures.BroadcastArchivedID)})
	require.NoError(t, err)
	b, ok := got.(*externalapi.BroadcastResource)
	require.Truef(t, ok, "got %T", got)
	segmentID, ok := b.SegmentId.Get()
	require.True(t, ok)
	assert.Equal(t, entityIDString(fixtures.SegmentArchivedID), segmentID)
}

func TestExternalBroadcastSegmentRefusesAForeignSegment(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()
	before := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).SegmentID

	got, err := c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{SegmentId: externalapi.NewNilEntityId(entityIDString(fixtures.SegmentGlobexID))},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsSetSegmentUnprocessableEntity{}, got)
	assert.Equal(t, before, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).SegmentID)
}
