package segments_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/segmentsaving"
)

const (
	// badCondition tests a Custom field the Workspace does not have.
	badCondition = `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"no_such_field","test":{"kind":"text","operator":"eq","value":"x"}}]}`
)

func TestCreateRejectsInvalidDefinition(t *testing.T) {
	env := testhelper.Setup(t)
	save := testhelper.Module[*segmentsaving.UseCase](env)
	ctx := context.Background()

	_, err := save.Create(ctx, env.DB.Scoped(fixtures.AcmeID), segments.CreateInput{Name: "Bad", Condition: new(testhelper.SegmentCondition(t, badCondition))})
	require.ErrorIs(t, err, segments.ErrInvalidCondition)

	persisted, err := env.DB.Scoped(fixtures.AcmeID).Segment().Query().Where(segment.Name("Bad")).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, persisted, "nothing persisted")
}

// A leaf that looks back no days is below the contract's minimum, whichever driver saves it.
func TestCreateRefusesAWindowOfNoDays(t *testing.T) {
	env := testhelper.Setup(t)
	save := testhelper.Module[*segmentsaving.UseCase](env)
	ctx := context.Background()
	def := `{"kind":"group","combinator":"and","entries":[` +
		`{"kind":"event","action":"signup","operator":"atLeast","times":1,"withinDays":0}]}`

	_, err := save.Create(ctx, env.DB.Scoped(fixtures.AcmeID), segments.CreateInput{Name: "No days", Condition: new(testhelper.SegmentCondition(t, def))})

	require.ErrorIs(t, err, segments.ErrInvalidCondition)
	assert.Contains(t, validation.Message(err), "withinDays must be at least 1")
}

func TestCreateAcceptsValidAndEmptyDefinition(t *testing.T) {
	env := testhelper.Setup(t)
	save := testhelper.Module[*segmentsaving.UseCase](env)
	ctx := context.Background()

	def := `{"kind":"group","combinator":"and","entries":[{"kind":"attribute","attribute":"email","test":{"kind":"text","operator":"contains","value":"x"}}]}`
	s, err := save.Create(ctx, env.DB.Scoped(fixtures.AcmeID), segments.CreateInput{Name: "Good", Condition: new(testhelper.SegmentCondition(t, def))})
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmeID, s.WorkspaceID)
	assert.JSONEq(t, def, testhelper.Document(t, s.Condition))

	_, err = save.Create(ctx, env.DB.Scoped(fixtures.AcmeID), segments.CreateInput{Name: "Empty", Condition: &conditiontree.Segment{Group: conditiontree.Empty()}})
	require.NoError(t, err)
	_, err = save.Create(ctx, env.DB.Scoped(fixtures.AcmeID), segments.CreateInput{Name: "Nil"})
	require.NoError(t, err)
}

func TestUpdateValidatesLikeCreate(t *testing.T) {
	env := testhelper.Setup(t)
	save := testhelper.Module[*segmentsaving.UseCase](env)
	ctx := context.Background()

	_, err := save.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID, segments.UpdateInput{Revision: env.SegmentRevision(ctx, t, fixtures.SegmentProPlanID), Condition: patch.Set(testhelper.SegmentCondition(t, badCondition))})
	require.ErrorIs(t, err, segments.ErrInvalidCondition)

	// An invalid definition is rejected and leaves the row untouched.
	_, err = save.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID, segments.UpdateInput{Revision: env.SegmentRevision(ctx, t, fixtures.SegmentProPlanID), Condition: patch.Set(testhelper.SegmentCondition(t, badCondition))})
	require.ErrorIs(t, err, segments.ErrInvalidCondition)
	s, err := env.DB.Segment.Get(ctx, fixtures.SegmentProPlanID)
	require.NoError(t, err)
	assert.NotEqual(t, testhelper.SegmentCondition(t, badCondition), s.Condition)

	got, err := save.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID, segments.UpdateInput{Revision: env.SegmentRevision(ctx, t, fixtures.SegmentProPlanID), Name: patch.Set("Renamed")})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", got.Name)
}

func TestUpdateIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	save := testhelper.Module[*segmentsaving.UseCase](env)

	_, err := save.Update(context.Background(), env.DB.Scoped(fixtures.GlobexID), fixtures.SegmentProPlanID, segments.UpdateInput{Revision: 1, Name: patch.Set("x")})
	require.ErrorIs(t, err, segments.ErrNotFound)
}

func TestPreviewAndCountAgree(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*segments.Module](env)
	ctx := context.Background()

	stored, err := env.DB.Segment.Get(ctx, fixtures.SegmentProPlanID)
	require.NoError(t, err)
	pred, err := condition.New(time.Now).Compile(ctx, env.DB.Scoped(fixtures.AcmeID), stored.Condition)
	require.NoError(t, err)
	want, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.AcmeID), pred).Count(ctx)
	require.NoError(t, err)
	require.Positive(t, want, "fixture segment should match someone")

	preview, err := m.Preview(ctx, env.DB.Scoped(fixtures.AcmeID), segments.PreviewInput{Condition: &stored.Condition})
	require.NoError(t, err)
	assert.Equal(t, want, preview.Count)
	assert.Nil(t, preview.Recipients, "no Channel was asked for")

	count, err := m.Count(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID)
	require.NoError(t, err)
	assert.Equal(t, want, count.Count)
}

// A Segment that counts opens leaves out a Contact whose address refused tracking, and its
// preview and count both say so (ADR 0123). Massive's sunset Segment holds Ann, who opened nothing;
// Bob and Cara opened, and stay out as before.
func TestPreviewAndCountReportTheContactsLeftOutForUnknownEngagement(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*segments.Module](env)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.MassiveID)
	stored, err := s.Segment().Get(ctx, fixtures.SegmentMassiveUnengagedID)
	require.NoError(t, err)
	before, err := m.Count(ctx, s, fixtures.SegmentMassiveUnengagedID)
	require.NoError(t, err)

	_, err = testhelper.Module[*engagementtracking.Module](env).Record(ctx, s, engagementtracking.RecordInput{
		Channel: channels.Email, Destination: fixtures.MassiveAnnEmail, State: engagementtracking.Denied, Provenance: engagementtracking.Manual,
	})
	require.NoError(t, err)
	preview, err := m.Preview(ctx, s, segments.PreviewInput{Condition: &stored.Condition})
	require.NoError(t, err)
	count, err := m.Count(ctx, s, fixtures.SegmentMassiveUnengagedID)
	require.NoError(t, err)

	assert.Equal(t, segments.Counts{Count: 1}, before, "Ann, while she is tracked")
	assert.Equal(t, segments.Counts{Count: 0, UnknownEngagement: 1}, preview)
	assert.Equal(t, preview, count)
}

func TestPreviewAndCountErrors(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*segments.Module](env)
	ctx := context.Background()

	_, err := m.Preview(ctx, env.DB.Scoped(fixtures.AcmeID), segments.PreviewInput{Condition: new(testhelper.SegmentCondition(t, badCondition))})
	require.ErrorIs(t, err, segments.ErrInvalidCondition)

	_, err = m.Preview(ctx, env.DB.Scoped(fixtures.AcmeID), segments.PreviewInput{Condition: new(testhelper.SegmentCondition(t, badCondition)), Channel: new(channels.Email)})
	require.ErrorIs(t, err, segments.ErrInvalidCondition, "a Channel asked for changes nothing about a bad condition")

	_, err = m.Count(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.SegmentProPlanID)
	require.ErrorIs(t, err, segments.ErrNotFound)
}

// With a Channel the preview also counts the recipients a Broadcast there would take: on Telegram
// the Contacts with an eligible Binding on the routed Integration. Duff's bot has two Contacts
// bound, one of them in two chats; a preview without a condition matches every Contact.
func TestPreviewCountsTheRecipientsOfABroadcastOnTheChannel(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*segments.Module](env)
	ctx := context.Background()
	ws := env.DB.Scoped(fixtures.DuffID)

	preview, err := m.Preview(ctx, ws, segments.PreviewInput{
		Channel: new(channels.Telegram), IntegrationID: new(int64(fixtures.IntegrationDuffTelegramID)),
	})

	require.NoError(t, err)
	require.NotNil(t, preview.Recipients)
	assert.Equal(t, 2, *preview.Recipients, "the two bound Contacts, whatever the chats each has")
	all, err := ws.Contact().Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, all, preview.Count, "no condition matches every Contact")
}

// A stored Segment "can be texted" counts and lists, live, the Contacts whose phone has an SMS
// Confirmation and no SMS opt-out (fixtures: Tyrell's eligibility matrix, where only "confirmed"
// has both), and its negation the rest of the Workspace.
func TestASegmentOfContactsThatCanBeTextedCountsAndListsThem(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*segments.Module](env)
	ctx := context.Background()
	ws := env.DB.Scoped(fixtures.TyrellID)
	listed := func(id int64) (int, []int64) {
		t.Helper()
		count, err := m.Count(ctx, ws, id)
		require.NoError(t, err)
		page, err := m.Contacts(ctx, ws, id, pagination.Params{Page: 1, PageSize: 100})
		require.NoError(t, err)
		ids := make([]int64, len(page.Items))
		for i, c := range page.Items {
			ids[i] = c.ID
		}
		return count.Count, ids
	}
	all, err := ws.Contact().Query().Count(ctx)
	require.NoError(t, err)

	count, ids := listed(fixtures.SegmentTyrellTextableID)
	assert.Equal(t, 1, count)
	assert.Equal(t, []int64{fixtures.TyrellMatrixConfirmedID}, ids)
	count, ids = listed(fixtures.SegmentTyrellUntextableID)
	assert.Equal(t, all-1, count)
	assert.Contains(t, ids, int64(fixtures.TyrellMatrixCleanID), "a phone without a Confirmation")
	assert.Contains(t, ids, int64(fixtures.TyrellMatrixUnsubEverythingID), "a confirmed phone that opted out")
	assert.Contains(t, ids, int64(fixtures.TyrellMatrixSuppressedID), "no phone")
	assert.NotContains(t, ids, int64(fixtures.TyrellMatrixConfirmedID))
}
