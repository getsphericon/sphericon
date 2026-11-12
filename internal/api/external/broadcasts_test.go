package external_test

import (
	"context"
	"net/http"
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

	b, err := c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
		Name:    "Spring sale",
		Content: testhelper.ExternalEmail(externalapi.EmailBroadcastContentInput{Subject: externalapi.NewOptString("Big news")}),
	})
	require.NoError(t, err)
	assert.Equal(t, externalapi.BroadcastStatusDraft, b.Status)
	assert.Equal(t, "Big news", testhelper.ExternalEmailOf(b).Subject)
	assert.False(t, b.SegmentId.IsSet(), "no Segment yet")

	_, err = c.BroadcastsGet(ctx, externalapi.BroadcastsGetParams{ID: b.ID})
	require.NoError(t, err)

	updated, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(b.ID))), Name: externalapi.NewOptName("Spring sale v2")},
		externalapi.BroadcastsUpdateParams{ID: b.ID})
	require.NoError(t, err)
	assert.Equal(t, "Spring sale v2", updated.Name)

	// A scheduled broadcast is no longer editable.
	_, err = c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(schedBroadcast))), Name: externalapi.NewOptName("x")},
		externalapi.BroadcastsUpdateParams{ID: schedBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusConflict)

	listed, err := c.BroadcastsList(ctx, externalapi.BroadcastsListParams{})
	require.NoError(t, err)
	assert.NotEmpty(t, listed.Items)
}

// The one pagination smoke of the external surface: the envelope is copied from the module's page.
// The /api body of an update is a merge patch with the fields /site has: a null clears the
// Segment and the From name, an absent key keeps them.
func TestExternalBroadcastsUpdateClearsANullAndKeepsAnAbsentField(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()
	// The draft has a Segment and a From name.
	params := externalapi.BroadcastsUpdateParams{ID: entityIDString(fixtures.BroadcastProSegmentID)}

	var none externalapi.OptNilString
	none.SetToNull()
	got, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(params.ID))), Content: testhelper.ExternalEmailEdit(externalapi.EmailBroadcastContentInput{FromName: none})}, params)
	require.NoError(t, err)
	assert.Empty(t, testhelper.ExternalEmailOf(got).FromName.Or(""), "a null clears the From name")
	assert.Equal(t, externalapi.NewOptNilEntityId(proSegment), got.SegmentId, "an absent Segment is kept")
}

func TestExternalBroadcastsListPaginates(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)

	page, err := c.BroadcastsList(context.Background(), externalapi.BroadcastsListParams{
		Page: externalapi.NewOptInt32(2), PageSize: externalapi.NewOptInt32(2),
	})
	require.NoError(t, err)
	assert.Len(t, page.Items, 2)
	assert.EqualValues(t, 2, page.Page)
	assert.EqualValues(t, 2, page.PageSize)
	assert.Greater(t, page.TotalItems, int32(4))
	assert.Greater(t, page.TotalPages, int32(2))
}

func TestExternalBroadcastSegmentIsASegmentOrEveryone(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()

	b, err := c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(draftBroadcast))), SegmentId: externalapi.NewNilEntityId(proSegment)},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	require.NoError(t, err)
	id, ok := b.SegmentId.Get()
	require.True(t, ok)
	assert.Equal(t, proSegment, id)
	assert.Equal(t, externalapi.BroadcastStatusDraft, b.Status, "setting a Segment never sends or schedules")

	cleared, err := c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(draftBroadcast))), SegmentId: externalapi.NilEntityId{Null: true}},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.False(t, cleared.SegmentId.IsSet())

	_, err = c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(draftBroadcast))), SegmentId: externalapi.NewNilEntityId(999999)},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(schedBroadcast))), SegmentId: externalapi.NewNilEntityId(proSegment)},
		externalapi.BroadcastsSetSegmentParams{ID: schedBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusConflict)
}

func TestExternalBroadcastTestSendGoesToOneAddressOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()

	err := c.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Email: externalapi.NewOptEmailAddress("qa@test.dev")},
		externalapi.BroadcastsTestSendParams{ID: draftBroadcast})
	require.NoError(t, err)

	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "qa@test.dev", msgs[0].To)
	assert.Equal(t, broadcast.StatusDraft, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status, "a test send does not touch the lifecycle")
}

func TestExternalBroadcastReport(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()

	r, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: failedBroadcast})
	require.NoError(t, err)
	assert.Equal(t, externalapi.BroadcastStatusFailed, r.Status)
	assert.EqualValues(t, 2, r.SkippedCount)
	assert.EqualValues(t, 3, r.FailedCount)

	sent, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: sentBroadcast})
	require.NoError(t, err)
	sr := sent
	// The figures are the recipients' (fixtures: 11 sent, 5 opened, 2 clicked).
	assert.EqualValues(t, 11, sr.RecipientsTotal)
	assert.EqualValues(t, 11, sr.SentCount)
	assert.EqualValues(t, 5, sr.OpenedCount.Value)
	assert.EqualValues(t, 2, sr.ClickedCount)
}

func TestExternalBroadcastsRequireScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	readOnly := env.ExternalScoped(t, "broadcasts:read")
	noScope := env.ExternalScoped(t, "contacts:read")

	// read scope reads...
	_, err := readOnly.BroadcastsGet(ctx, externalapi.BroadcastsGetParams{ID: draftBroadcast})
	require.NoError(t, err)
	_, err = readOnly.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: sentBroadcast})
	require.NoError(t, err)

	// ...but cannot write, set the Segment or test-send.
	_, err = readOnly.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{Name: "x", Content: testhelper.ExternalEmail(externalapi.EmailBroadcastContentInput{})})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	_, err = readOnly.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(draftBroadcast)))}, externalapi.BroadcastsUpdateParams{ID: draftBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	_, err = readOnly.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(draftBroadcast))), SegmentId: externalapi.NilEntityId{Null: true}},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	err = readOnly.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Email: externalapi.NewOptEmailAddress("qa@test.dev")}, externalapi.BroadcastsTestSendParams{ID: draftBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	assert.Empty(t, env.CustomerMail.Messages())

	// A token without any broadcast scope reads nothing.
	_, err = noScope.BroadcastsList(ctx, externalapi.BroadcastsListParams{})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	_, err = noScope.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: sentBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}

func TestExternalBroadcastScheduleNeedsTheSendScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	when := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	body := &externalapi.ScheduleBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(draftBroadcast))), ScheduledAt: externalapi.Timestamp(when)}

	// Authoring scopes are not enough to send, in either direction.
	author := env.ExternalScoped(t, authorScopes...)
	_, err := author.BroadcastsSchedule(ctx, body, externalapi.BroadcastsScheduleParams{ID: draftBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	_, err = author.BroadcastsUnschedule(ctx, externalapi.BroadcastsUnscheduleParams{ID: schedBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	assert.Equal(t, broadcast.StatusDraft, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status)
	assert.Equal(t, broadcast.StatusScheduled, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastScheduledID).Status)

	// emails:send is a different lock.
	emailer := env.ExternalScoped(t, "emails:send")
	_, err = emailer.BroadcastsSchedule(ctx, body, externalapi.BroadcastsScheduleParams{ID: draftBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	// broadcasts:send alone schedules and unschedules.
	sender := env.ExternalScoped(t, "broadcasts:send")
	b, err := sender.BroadcastsSchedule(ctx, body, externalapi.BroadcastsScheduleParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.Equal(t, externalapi.BroadcastStatusScheduled, b.Status)
	sched, ok := b.ScheduledAt.Get()
	require.True(t, ok)
	assert.True(t, when.Equal(time.Time(sched)))

	b, err = sender.BroadcastsUnschedule(ctx, externalapi.BroadcastsUnscheduleParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.Equal(t, externalapi.BroadcastStatusDraft, b.Status)
	assert.Nil(t, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).ScheduledAt)
}

func TestExternalBroadcastScheduleRefusesWhatCannotBeScheduled(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "broadcasts:send")
	body := &externalapi.ScheduleBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(sentBroadcast))), ScheduledAt: externalapi.Timestamp(time.Now().Add(time.Hour))}

	_, err := c.BroadcastsSchedule(ctx, body, externalapi.BroadcastsScheduleParams{ID: sentBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity, "a sent broadcast is history")
	_, err = c.BroadcastsSchedule(ctx, body, externalapi.BroadcastsScheduleParams{ID: 999999})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)

	_, err = c.BroadcastsUnschedule(ctx, externalapi.BroadcastsUnscheduleParams{ID: draftBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity, "nothing to unschedule on a draft")
	_, err = c.BroadcastsUnschedule(ctx, externalapi.BroadcastsUnscheduleParams{ID: 999999})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
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
	for _, item := range list.Items {
		assert.NotEqual(t, foreignID, item.ID, "another tenant's broadcast leaked")
	}

	_, err = c.BroadcastsGet(ctx, externalapi.BroadcastsGetParams{ID: foreignID})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
	_, err = c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(foreignID))), Name: externalapi.NewOptName("hijack")}, externalapi.BroadcastsUpdateParams{ID: foreignID})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
	_, err = c.BroadcastsArchive(ctx, externalapi.BroadcastsArchiveParams{ID: foreignID})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
	_, err = c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: foreignID})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
	err = c.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Email: externalapi.NewOptEmailAddress("qa@test.dev")}, externalapi.BroadcastsTestSendParams{ID: foreignID})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
	assert.Empty(t, env.CustomerMail.Messages())
	_, err = c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(foreignID))), SegmentId: externalapi.NilEntityId{Null: true}}, externalapi.BroadcastsSetSegmentParams{ID: foreignID})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)

	// Another workspace's segment cannot be the Segment of our broadcast.
	_, err = c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(draftBroadcast))), SegmentId: externalapi.NewNilEntityId(externalapi.EntityId(foreignSegID))},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)

	assert.Equal(t, fixtures.BroadcastGlobexName, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastGlobexID).Name, "the foreign broadcast is untouched")
}

// An archived Segment leaves new work: a draft cannot be pointed at it, and a refused call
// changes nothing.
func TestExternalBroadcastSegmentRefusesAnArchivedSegment(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()
	before := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)

	_, err := c.BroadcastsSetSegment(ctx,
		&externalapi.SetBroadcastSegmentInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(draftBroadcast))), SegmentId: externalapi.NewNilEntityId(entityIDString(fixtures.SegmentArchivedID))},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	problem := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, problem.Detail.Value, fixtures.SegmentArchivedName, "the refusal names the archived Segment")

	after := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, before.SegmentID, after.SegmentID)
}

// A Broadcast that already points at an archived Segment keeps showing it.
func TestExternalBroadcastKeepsShowingItsArchivedSegment(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "broadcasts:read")
	ctx := context.Background()

	b, err := c.BroadcastsGet(ctx, externalapi.BroadcastsGetParams{ID: entityIDString(fixtures.BroadcastArchivedID)})
	require.NoError(t, err)
	segmentID, ok := b.SegmentId.Get()
	require.True(t, ok)
	assert.Equal(t, entityIDString(fixtures.SegmentArchivedID), segmentID)
}

func TestExternalBroadcastSegmentRefusesAForeignSegment(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()
	before := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).SegmentID

	_, err := c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(draftBroadcast))), SegmentId: externalapi.NewNilEntityId(entityIDString(fixtures.SegmentGlobexID))},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, before, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).SegmentID)
}
