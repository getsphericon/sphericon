package app_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestCommandAppResolvesOneWorkspaceOrInstanceForJournalInspection(t *testing.T) {
	a, _ := newCommandApp(t)
	bus := testhelper.BusOn(t, sqlDBOn(t, os.Getenv("SPHERICON_DATABASE_URL")))
	require.NoError(t, bus.Publish(t.Context(), &events.SecondFactorRequired{WorkspaceID: fixtures.AcmeID}))
	require.NoError(t, bus.Publish(t.Context(), &events.PasskeyChange{Action: events.NameUserPasskeyAdded}))
	workspace, err := a.EventJournal(t.Context(), app.EventJournalRequest{Workspace: fixtures.AcmePublicID})
	require.NoError(t, err)
	require.Len(t, workspace.Entries, 1)
	assert.Equal(t, events.NameWorkspaceSecondFactorRequired, workspace.Entries[0].Name)
	instance, err := a.EventJournal(t.Context(), app.EventJournalRequest{Instance: true})
	require.NoError(t, err)
	require.Len(t, instance.Entries, 1)
	assert.Equal(t, events.NameUserPasskeyAdded, instance.Entries[0].Name)
	for _, request := range []app.EventJournalRequest{
		{}, {Workspace: fixtures.AcmePublicID, Instance: true}, {Workspace: "NoSuchId"},
	} {
		_, err := a.EventJournal(t.Context(), request)
		require.Error(t, err)
	}
}
