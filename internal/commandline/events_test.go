package commandline_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/commandline"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func eventHistory(env *testhelper.TestEnv) commandline.EventHistory {
	return commandline.EventHistory{Workspaces: workspaces.New(env.DB, env.Bus, time.Now), Bus: env.Bus}
}

// unsubscribeReplay asks for the Broadcast unsubscribe consumer's jobs of the minute from at.
func unsubscribeReplay(at time.Time) events.ReplayRequest {
	return events.ReplayRequest{Consumer: jobs.BroadcastUnsubscribeEventArgs{}.Kind(), From: at, Before: at.Add(time.Minute)}
}

func listed(t *testing.T, out *bytes.Buffer) events.JournalPage {
	t.Helper()
	var page events.JournalPage
	require.NoError(t, json.Unmarshal(out.Bytes(), &page))
	return page
}

func TestListEventsReadsTheWorkspaceAnOperatorNamesByItsID(t *testing.T) {
	env := testhelper.Setup(t)
	var out bytes.Buffer

	err := eventHistory(env).List(t.Context(), &out, commandline.JournalOf{Workspace: fixtures.KramericaPublicID}, events.JournalQuery{Name: fixtures.JournalOldestName})

	require.NoError(t, err)
	page := listed(t, &out)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, fixtures.JournalOtherWorkspaceEventID, page.Entries[0].EventID)
}

func TestListEventsOfTheInstanceReadsOnlyItsOwnFacts(t *testing.T) {
	env := testhelper.Setup(t)
	var out bytes.Buffer

	err := eventHistory(env).List(t.Context(), &out, commandline.JournalOf{Instance: true}, events.JournalQuery{})

	require.NoError(t, err)
	page := listed(t, &out)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, fixtures.JournalInstanceEventID, page.Entries[0].EventID)
}

func TestListEventsRefusesAnUnknownWorkspaceInsteadOfReadingTheInstance(t *testing.T) {
	env := testhelper.Setup(t)
	var out bytes.Buffer

	unknown := eventHistory(env).List(t.Context(), &out, commandline.JournalOf{Workspace: "NoSuchId"}, events.JournalQuery{})
	empty := eventHistory(env).List(t.Context(), &out, commandline.JournalOf{}, events.JournalQuery{})

	require.ErrorIs(t, unknown, workspaces.ErrNotFound)
	require.ErrorIs(t, empty, workspaces.ErrNotFound)
	assert.Empty(t, out.String())
}

func TestReplayEventsReportsWhatItMatchedAndQueued(t *testing.T) {
	at := time.Date(2026, 11, 8, 2, 0, 0, 0, time.UTC)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	require.NoError(t, env.Bus.Publish(t.Context(), &events.MessageUnsubscribed{WorkspaceID: fixtures.AcmeID, OutboundMessageID: fixtures.OutboundMessageTrackingSampleID, ObservedAt: at}))
	var out bytes.Buffer

	err := eventHistory(env).Replay(t.Context(), &out, fixtures.AcmePublicID, unsubscribeReplay(at))

	require.NoError(t, err)
	assert.Equal(t, "matched 1; enqueued 1\n", out.String())
}

func TestReplayEventsOfAnUnknownWorkspaceIsUnavailable(t *testing.T) {
	at := time.Date(2026, 11, 8, 2, 0, 0, 0, time.UTC)
	env := testhelper.Setup(t)
	var out bytes.Buffer

	err := eventHistory(env).Replay(t.Context(), &out, "NoSuchId", unsubscribeReplay(at))

	require.ErrorIs(t, err, events.ErrReplayWorkspaceUnavailable)
	require.ErrorContains(t, err, `workspace "NoSuchId"`)
	assert.Empty(t, out.String())
}

func TestReplayEventsOfAWorkspaceInItsDeletionWindowIsUnavailable(t *testing.T) {
	at := time.Date(2026, 11, 8, 2, 0, 0, 0, time.UTC)
	env := testhelper.Setup(t)
	var out bytes.Buffer

	err := eventHistory(env).Replay(t.Context(), &out, fixtures.BluthPublicID, unsubscribeReplay(at))

	require.ErrorIs(t, err, events.ErrReplayWorkspaceUnavailable)
	require.ErrorContains(t, err, `workspace "`+fixtures.BluthPublicID+`"`)
	assert.Empty(t, out.String())
}
