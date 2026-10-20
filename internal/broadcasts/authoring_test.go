package broadcasts_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func canceled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// failBroadcastReads makes every Broadcast query fail (writes are unaffected).
func failBroadcastReads(env *testhelper.TestEnv) {
	env.DB.Broadcast.Intercept(ent.InterceptFunc(func(ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(context.Context, ent.Query) (ent.Value, error) {
			return nil, errors.New("read refused")
		})
	}))
}

func TestListIsNewestFirstAndWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(&recorder{})

	page, err := m.List(context.Background(), env.DB.Scoped(fixtures.AcmeID), pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.Len(t, page.Items, page.TotalItems)
	require.Greater(t, page.TotalItems, 2)
	for _, b := range page.Items {
		assert.EqualValues(t, fixtures.AcmeID, b.WorkspaceID)
	}
	ids := lo.Map(page.Items, func(b *ent.Broadcast, _ int) int64 { return b.ID })
	assert.IsDecreasing(t, ids, "newest first")
}

func TestCreateReportsErrors(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := broadcasts.New(&recorder{}).Create(canceled(), env.DB.Scoped(fixtures.AcmeID), broadcasts.Fields{})
	assert.Error(t, err)
}

func TestCreateMakesADraftWithTheGivenFields(t *testing.T) {
	env := testhelper.Setup(t)
	name, subject, body, from := "N", "S", "<p></p>", "a@b.com"

	b, err := broadcasts.New(&recorder{}).Create(context.Background(), env.DB.Scoped(fixtures.AcmeID),
		broadcasts.Fields{Name: &name, Subject: &subject, Body: &body, FromEmail: &from})
	require.NoError(t, err)
	assert.Equal(t, broadcast.StatusDraft, b.Status)
	assert.Equal(t, "N", b.Name)
	assert.Equal(t, "S", b.Subject)
	assert.Equal(t, "<p></p>", b.Body)
	require.NotNil(t, b.FromEmail)
	assert.Equal(t, "a@b.com", *b.FromEmail)
}

func TestUpdateOnlyEditsDraftsAndKeepsUnsetFields(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(&recorder{})
	ctx := context.Background()
	subject := "New subject"

	before := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	b, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID, broadcasts.Fields{Subject: &subject})
	require.NoError(t, err)
	assert.Equal(t, "New subject", b.Subject)
	assert.Equal(t, before.Name, b.Name, "unset fields keep the stored value")

	_, err = m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastScheduledID, broadcasts.Fields{Subject: &subject})
	require.ErrorIs(t, err, broadcasts.ErrNotDraft)
	_, err = m.Update(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.BroadcastDraftID, broadcasts.Fields{Subject: &subject})
	require.ErrorIs(t, err, broadcasts.ErrNotFound)
	_, err = m.Update(canceled(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID, broadcasts.Fields{Subject: &subject})
	assert.Error(t, err)
}

func TestSetAudience(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(&recorder{})
	ctx := context.Background()
	seg := int64(fixtures.SegmentProPlanID)

	b, err := m.SetAudience(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID, &seg)
	require.NoError(t, err)
	require.NotNil(t, b.SegmentID)
	assert.Equal(t, seg, *b.SegmentID)

	b, err = m.SetAudience(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID, nil)
	require.NoError(t, err)
	assert.Nil(t, b.SegmentID, "nil audience means all active contacts")

	globexSeg := int64(fixtures.SegmentGlobexID)
	_, err = m.SetAudience(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID, &globexSeg)
	require.ErrorIs(t, err, broadcasts.ErrSegmentNotFound, "another workspace's segment is not usable")

	_, err = m.SetAudience(ctx, env.DB.Scoped(fixtures.AcmeID), 987654, &globexSeg)
	require.ErrorIs(t, err, broadcasts.ErrNotFound, "an unknown broadcast is reported before the segment")

	_, err = m.SetAudience(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastScheduledID, &seg)
	require.ErrorIs(t, err, broadcasts.ErrNotDraft)

	_, err = m.SetAudience(canceled(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID, &seg)
	require.Error(t, err, "segment lookup failure")
	_, err = m.SetAudience(canceled(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID, nil)
	assert.Error(t, err, "update failure")
}

func TestDeleteDraft(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(&recorder{})
	ctx := context.Background()

	require.NoError(t, m.DeleteDraft(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID))
	assert.False(t, env.DB.Broadcast.Query().Where(broadcast.ID(fixtures.BroadcastDraftID)).ExistX(ctx))

	require.ErrorIs(t, m.DeleteDraft(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastScheduledID), broadcasts.ErrNotDraft)
	require.ErrorIs(t, m.DeleteDraft(ctx, env.DB.Scoped(fixtures.AcmeID), 987654), broadcasts.ErrNotFound)
	assert.Error(t, m.DeleteDraft(canceled(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastEmptyAudienceID))
}

func TestReport(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(&recorder{})
	ctx := context.Background()

	// The figures are the rows': 11 recipients of the sent Broadcast, all sent, 5 opened and 2
	// clicked in the fixtures; one more unsubscribes now.
	env.DB.BroadcastRecipient.UpdateOneID(fixtures.BroadcastRecipientSentID).SetUnsubscribedAt(time.Now()).ExecX(ctx)
	r, err := m.Report(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastSentID)
	require.NoError(t, err)
	assert.Equal(t, broadcast.StatusSent, r.Status)
	assert.Equal(t, 11, r.Recipients)
	assert.Equal(t, 11, r.Sent)
	assert.Equal(t, 5, r.Opened)
	assert.Equal(t, 2, r.Clicked)
	assert.Equal(t, 1, r.Unsubscribed)
	assert.InDelta(t, 5.0/11, r.OpenRate, 1e-6)
	assert.InDelta(t, 2.0/11, r.ClickRate, 1e-6)

	zero, err := m.Report(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID)
	require.NoError(t, err)
	assert.Zero(t, zero.OpenRate, "no sends means a zero rate, not NaN")
	assert.Zero(t, zero.ClickRate)

	_, err = m.Report(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.BroadcastSentID)
	assert.ErrorIs(t, err, broadcasts.ErrNotFound)
}

func TestGetUnknownAndErrors(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(&recorder{})

	_, err := m.Get(context.Background(), env.DB.Scoped(fixtures.AcmeID), 987654)
	require.ErrorIs(t, err, broadcasts.ErrNotFound)
	_, err = m.Get(canceled(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID)
	require.Error(t, err)
	assert.NotErrorIs(t, err, broadcasts.ErrNotFound)
}

func TestTransitionStoreErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("claim update fails", func(t *testing.T) {
		env := testhelper.Setup(t)
		_, err := broadcasts.New(&recorder{}).Send(canceled(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID)
		assert.Error(t, err)
	})

	t.Run("claim existence check fails", func(t *testing.T) {
		env := testhelper.Setup(t)
		failBroadcastReads(env)
		_, err := broadcasts.New(&recorder{}).Send(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastSentID)
		assert.ErrorContains(t, err, "read refused")
	})

	t.Run("unschedule update fails", func(t *testing.T) {
		env := testhelper.Setup(t)
		_, err := broadcasts.New(&recorder{}).Unschedule(canceled(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastScheduledID)
		assert.Error(t, err)
	})

	t.Run("unschedule existence check fails", func(t *testing.T) {
		env := testhelper.Setup(t)
		failBroadcastReads(env)
		_, err := broadcasts.New(&recorder{}).Unschedule(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID)
		assert.ErrorContains(t, err, "read refused")
	})
}

// Another Workspace's rows are invisible through the scoped client: a foreign
// Broadcast is not found, and a foreign Segment cannot become the audience.
func TestAnotherWorkspacesBroadcastAndSegmentAreRefused(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(&recorder{})
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)

	_, err := m.Get(ctx, acme, fixtures.BroadcastGlobexID)
	require.ErrorIs(t, err, broadcasts.ErrNotFound)
	_, err = m.Send(ctx, acme, fixtures.BroadcastGlobexID)
	require.ErrorIs(t, err, broadcasts.ErrNotFound)

	_, err = m.SetAudience(ctx, acme, fixtures.BroadcastDraftID, new(int64(fixtures.SegmentGlobexID)))
	assert.ErrorIs(t, err, broadcasts.ErrSegmentNotFound)
}

func TestDeleteRemovesABroadcastOfAnyStatusAndIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(&recorder{})
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	require.NoError(t, m.Delete(ctx, s, fixtures.BroadcastFailedID))
	_, err := m.Get(ctx, s, fixtures.BroadcastFailedID)
	require.ErrorIs(t, err, broadcasts.ErrNotFound)

	require.ErrorIs(t, m.Delete(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.BroadcastDraftID), broadcasts.ErrNotFound,
		"another Workspace's Broadcast is not found")
	_, err = m.Get(ctx, s, fixtures.BroadcastDraftID)
	assert.NoError(t, err)
}
