package eventhistory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/eventhistory"
)

func history(env *testhelper.TestEnv) *eventhistory.UseCase {
	return eventhistory.New(workspaces.New(env.DB, env.Bus, time.Now), env.Bus)
}

func TestListReadsTheWorkspaceAnOperatorNamesByItsID(t *testing.T) {
	env := testhelper.Setup(t)

	page, err := history(env).List(t.Context(), eventhistory.Of{Workspace: fixtures.KramericaPublicID}, events.JournalQuery{Name: fixtures.JournalOldestName})

	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, fixtures.JournalOtherWorkspaceEventID, page.Entries[0].EventID)
}

func TestListOfTheInstanceReadsOnlyItsOwnFacts(t *testing.T) {
	env := testhelper.Setup(t)

	page, err := history(env).List(t.Context(), eventhistory.Of{Instance: true}, events.JournalQuery{})

	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, fixtures.JournalInstanceEventID, page.Entries[0].EventID)
}

func TestListRefusesAnUnknownWorkspaceInsteadOfReadingTheInstance(t *testing.T) {
	env := testhelper.Setup(t)

	_, unknown := history(env).List(t.Context(), eventhistory.Of{Workspace: "NoSuchId"}, events.JournalQuery{})
	_, empty := history(env).List(t.Context(), eventhistory.Of{}, events.JournalQuery{})

	require.ErrorIs(t, unknown, workspaces.ErrNotFound)
	require.ErrorIs(t, empty, workspaces.ErrNotFound)
}

func TestReplayOfAnUnknownWorkspaceIsUnavailable(t *testing.T) {
	at := time.Date(2026, 11, 8, 2, 0, 0, 0, time.UTC)
	env := testhelper.Setup(t)

	_, err := history(env).Replay(t.Context(), "NoSuchId", events.ReplayRequest{Consumer: jobs.BroadcastUnsubscribeEventArgs{}.Kind(), From: at, Before: at.Add(time.Minute)})

	require.ErrorIs(t, err, events.ErrReplayWorkspaceUnavailable)
}

func TestReplayOfAWorkspaceInItsDeletionWindowIsUnavailable(t *testing.T) {
	at := time.Date(2026, 11, 8, 2, 0, 0, 0, time.UTC)
	env := testhelper.Setup(t)

	_, err := history(env).Replay(t.Context(), fixtures.BluthPublicID, events.ReplayRequest{Consumer: jobs.BroadcastUnsubscribeEventArgs{}.Kind(), From: at, Before: at.Add(time.Minute)})

	require.ErrorIs(t, err, events.ErrReplayWorkspaceUnavailable)
}
