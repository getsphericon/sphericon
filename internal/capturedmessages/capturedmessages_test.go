package capturedmessages_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/internal/capturedmessages"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func captureOne(t *testing.T, c messaging.Capturer, subject string) {
	t.Helper()
	require.NoError(t, c.Capture(t.Context(), messaging.Captured{
		Message: messaging.EmailMessage{To: "ada@example.com", From: "hello@globex.test", Subject: subject, HTML: "<p>x</p>", Text: "x"},
	}))
}

func TestCapturingKeepsOnlyTheNewestMessagesOfTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.GlobexID)
	c := capturedmessages.New(3).Capturer(s, fixtures.IntegrationGlobexInboxID)
	require.Equal(t, 2, s.CapturedMessage().Query().CountX(t.Context()), "the fixture holds two")

	captureOne(t, c, "third")
	captureOne(t, c, "fourth")

	left := s.CapturedMessage().Query().Order(capturedmessage.ByCreatedAt(), capturedmessage.ByID()).AllX(t.Context())
	require.Len(t, left, 3)
	subjects := []string{left[0].Subject, left[1].Subject, left[2].Subject}
	assert.Equal(t, []string{fixtures.CapturedMessageGlobexWelcomeSubject, "third", "fourth"}, subjects,
		"the oldest went first; the newest of the fixture stays")
}

func TestCappingOneWorkspaceLeavesAnotherUntouched(t *testing.T) {
	env := testhelper.Setup(t)
	other := env.DB.Scoped(fixtures.UmbrellaID).CapturedMessage().Query().CountX(t.Context())
	require.Positive(t, other)
	c := capturedmessages.New(1).Capturer(env.DB.Scoped(fixtures.GlobexID), fixtures.IntegrationGlobexInboxID)

	captureOne(t, c, "only")

	assert.Equal(t, 1, env.DB.Scoped(fixtures.GlobexID).CapturedMessage().Query().CountX(t.Context()))
	assert.Equal(t, other, env.DB.Scoped(fixtures.UmbrellaID).CapturedMessage().Query().CountX(t.Context()))
}

// Concurrent sends each insert and trim on their own connection; the last trim sees every committed
// insert, so the Workspace ends at the cap. The other tests ride one go-txdb transaction, which
// serializes statements, so this one runs on a database of its own.
func TestConcurrentCapturesNeverLeaveMoreThanTheCap(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	sqlDB.SetMaxOpenConns(8)
	client := db.NewEntClient(sqlDB)
	s := client.Scoped(fixtures.GlobexID)
	const kept = 5
	c := capturedmessages.New(kept).Capturer(s, fixtures.IntegrationGlobexInboxID)

	var wg sync.WaitGroup
	errs := make([]error, 24)
	for i := range errs {
		wg.Go(func() {
			errs[i] = c.Capture(t.Context(), messaging.Captured{
				Message: messaging.EmailMessage{To: "ada@example.com", From: "hello@globex.test", Subject: fmt.Sprintf("m%d", i), HTML: "<p>x</p>", Text: "x"},
			})
		})
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, kept, s.CapturedMessage().Query().CountX(t.Context()))
}

func TestPurgeDeletesMessagesPastTheRetentionWindowAcrossWorkspaces(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	before := env.DB.CapturedMessage.Query().CountX(ctx)

	deleted, err := capturedmessages.Purge(ctx, env.DB, 7*24*time.Hour, 1)

	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted, "in batches of one, every expired message still goes")
	assert.Equal(t, before-2, env.DB.CapturedMessage.Query().CountX(ctx))
	assert.False(t, env.DB.CapturedMessage.Query().Where(capturedmessage.IDIn(
		fixtures.CapturedMessageUmbrellaExpiredOneID, fixtures.CapturedMessageUmbrellaExpiredTwoID)).ExistX(ctx))
	assert.True(t, env.DB.CapturedMessage.Query().Where(capturedmessage.ID(fixtures.CapturedMessageAcmeArchivedInboxID)).ExistX(ctx), "six days old, inside the window")
}

func TestPurgeFollowsTheWindowItIsGiven(t *testing.T) {
	env := testhelper.Setup(t)
	window := 2 * 24 * time.Hour
	older := env.DB.CapturedMessage.Query().Where(capturedmessage.CreatedAtLT(time.Now().Add(-window))).CountX(t.Context())
	require.Greater(t, older, 4, "the fixtures hold messages of more than two days old")

	deleted, err := capturedmessages.Purge(t.Context(), env.DB, window, 100)

	require.NoError(t, err)
	assert.EqualValues(t, older, deleted, "every message older than the window goes; the newer ones stay")
	assert.Positive(t, env.DB.CapturedMessage.Query().CountX(t.Context()))
}

func TestPurgeWithAZeroWindowDeletesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	before := env.DB.CapturedMessage.Query().CountX(t.Context())

	deleted, err := capturedmessages.Purge(t.Context(), env.DB, 0, 10)

	require.NoError(t, err)
	assert.Zero(t, deleted)
	assert.Equal(t, before, env.DB.CapturedMessage.Query().CountX(t.Context()))
}
