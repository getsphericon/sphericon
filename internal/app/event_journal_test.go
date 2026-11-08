package app_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

func TestCommandAppResolvesOneWorkspaceOrInstanceForJournalInspection(t *testing.T) {
	a, _ := newCommandApp(t)
	workspace, err := a.EventJournal(t.Context(), app.EventJournalRequest{
		Workspace: fixtures.GlobexPublicID, Query: events.JournalQuery{Name: fixtures.JournalOldestName},
	})
	require.NoError(t, err)
	require.Len(t, workspace.Entries, 4)
	assert.Equal(t, fixtures.JournalLatestTieEventID, workspace.Entries[0].EventID)
	instance, err := a.EventJournal(t.Context(), app.EventJournalRequest{Instance: true})
	require.NoError(t, err)
	require.Len(t, instance.Entries, 1)
	assert.Equal(t, fixtures.JournalInstanceEventID, instance.Entries[0].EventID)
	for _, request := range []app.EventJournalRequest{
		{}, {Workspace: fixtures.GlobexPublicID, Instance: true}, {Workspace: "NoSuchId"},
	} {
		_, err := a.EventJournal(t.Context(), request)
		require.Error(t, err)
	}
}
