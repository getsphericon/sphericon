package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestPublishedJournalExposesMetadataWithoutPayloadOrNewWork(t *testing.T) {
	at := time.Date(2026, 11, 8, 4, 0, 0, 0, time.UTC)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	require.NoError(t, env.Bus.Publish(t.Context(), &events.EmailEngagement{
		Action: events.NameEmailOpened, WorkspaceID: fixtures.AcmeID,
		OutboundMessageID: fixtures.OutboundMessageTrackingSampleID, ContactID: fixtures.ContactAliceID,
		Email: fixtures.ContactAliceEmail, ObservedAt: at.Add(-time.Minute),
	}))
	before := env.JobsOf(t, jobs.MessageObservationArgs{}.Kind())
	page, err := env.Bus.WorkspaceJournal(t.Context(), env.DB.Scoped(fixtures.AcmeID), events.JournalQuery{Name: events.NameEmailOpened})
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	entry := page.Entries[0]
	assert.NotEmpty(t, entry.EventID)
	assert.Equal(t, events.NameEmailOpened, entry.Name)
	assert.Equal(t, 1, entry.Version)
	assert.True(t, at.Equal(entry.RecordedAt))
	assert.True(t, at.Add(-time.Minute).Equal(entry.OccurredAt))
	assert.Equal(t, []string{jobs.MessageObservationArgs{}.Kind()}, entry.RetainedConsumers)
	assert.False(t, page.Truncated)
	assert.Equal(t, before, env.JobsOf(t, jobs.MessageObservationArgs{}.Kind()), "inspection queues no report work")
	encoded, err := json.Marshal(page)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), fixtures.ContactAliceEmail)
	assert.NotContains(t, string(encoded), "workspaceId")
	assert.NotContains(t, string(encoded), "outboundMessageId")
	assert.NotContains(t, string(encoded), "args")
}

func TestPublishedWorkspaceAndInstanceHistoryStaySeparate(t *testing.T) {
	env := testhelper.Setup(t)
	for _, fact := range []events.DomainEvent{
		&events.SecondFactorRequired{WorkspaceID: fixtures.AcmeID},
		&events.SecondFactorRequired{WorkspaceID: fixtures.InitechID},
		&events.LinkRequested{Action: events.NameUserPasswordResetRequested, Email: "private@example.com", BrowserHash: "browser-secret"},
	} {
		require.NoError(t, env.Bus.Publish(t.Context(), fact))
	}
	acme, err := env.Bus.WorkspaceJournal(t.Context(), env.DB.Scoped(fixtures.AcmeID), events.JournalQuery{})
	require.NoError(t, err)
	other, err := env.Bus.WorkspaceJournal(t.Context(), env.DB.Scoped(fixtures.InitechID), events.JournalQuery{})
	require.NoError(t, err)
	instance, err := env.Bus.InstanceJournal(t.Context(), events.JournalQuery{})
	require.NoError(t, err)
	require.Len(t, acme.Entries, 1)
	require.Len(t, other.Entries, 1)
	require.Len(t, instance.Entries, 1)
	assert.NotEqual(t, acme.Entries[0].EventID, other.Entries[0].EventID)
	assert.Equal(t, events.NameUserPasswordResetRequested, instance.Entries[0].Name)
	assert.Empty(t, instance.Entries[0].RetainedConsumers)
	encoded, err := json.Marshal(instance)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private@example.com")
	assert.NotContains(t, string(encoded), "browser-secret")
	missing, err := env.Bus.WorkspaceJournal(t.Context(), env.DB.Scoped(fixtures.InitechID), events.JournalQuery{EventID: acme.Entries[0].EventID})
	require.NoError(t, err)
	assert.Empty(t, missing.Entries, "an event id cannot cross the selected scope")
}

func TestJournalFiltersByRecordedTimeAndExactNameAndID(t *testing.T) {
	at := time.Date(2026, 11, 8, 4, 0, 0, 0, time.UTC)
	now := at
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return now }))
	s := env.DB.Scoped(fixtures.AcmeID)
	for range 3 {
		require.NoError(t, env.Bus.Publish(t.Context(), &events.SecondFactorRequired{WorkspaceID: fixtures.AcmeID}))
		now = now.Add(time.Minute)
	}
	require.NoError(t, env.Bus.Publish(t.Context(), &events.InvitationCreated{WorkspaceID: fixtures.AcmeID, Email: "invitee@example.com", Token: "secret-token"}))
	all, err := env.Bus.WorkspaceJournal(t.Context(), s, events.JournalQuery{Name: events.NameWorkspaceSecondFactorRequired})
	require.NoError(t, err)
	require.Len(t, all.Entries, 3)
	assert.True(t, at.Add(2*time.Minute).Equal(all.Entries[0].RecordedAt))
	assert.True(t, at.Equal(all.Entries[2].RecordedAt))
	page, err := env.Bus.WorkspaceJournal(t.Context(), s, events.JournalQuery{
		Name: events.NameWorkspaceSecondFactorRequired, From: at, Before: at.Add(2 * time.Minute),
	})
	require.NoError(t, err)
	assert.Equal(t, all.Entries[1:], page.Entries, "inclusive start and exclusive end use recorded time")
	exact, err := env.Bus.WorkspaceJournal(t.Context(), s, events.JournalQuery{EventID: all.Entries[1].EventID})
	require.NoError(t, err)
	assert.Equal(t, all.Entries[1:2], exact.Entries)
	wrongName, err := env.Bus.WorkspaceJournal(t.Context(), s, events.JournalQuery{Name: events.NameInvitationCreated, EventID: all.Entries[1].EventID})
	require.NoError(t, err)
	assert.Empty(t, wrongName.Entries, "filters combine rather than widening the selection")
}

func TestJournalBoundsOutputAndBreaksEqualTimeTiesByInsertion(t *testing.T) {
	at := time.Date(2026, 11, 8, 4, 0, 0, 0, time.UTC)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	s := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, env.Bus.Publish(t.Context(), &events.SecondFactorRequired{WorkspaceID: fixtures.AcmeID}))
	first, err := env.Bus.WorkspaceJournal(t.Context(), s, events.JournalQuery{})
	require.NoError(t, err)
	require.Len(t, first.Entries, 1)
	require.NoError(t, env.Bus.Publish(t.Context(), &events.SecondFactorRequired{WorkspaceID: fixtures.AcmeID}))
	bounded, err := env.Bus.WorkspaceJournal(t.Context(), s, events.JournalQuery{Limit: 1})
	require.NoError(t, err)
	require.Len(t, bounded.Entries, 1)
	assert.True(t, bounded.Truncated)
	assert.NotEqual(t, first.Entries[0].EventID, bounded.Entries[0].EventID)
	full, err := env.Bus.WorkspaceJournal(t.Context(), s, events.JournalQuery{Limit: 2})
	require.NoError(t, err)
	require.Len(t, full.Entries, 2)
	assert.False(t, full.Truncated)
	assert.Equal(t, bounded.Entries[0], full.Entries[0])
	assert.Equal(t, first.Entries[0], full.Entries[1])
}

func TestJournalRefusesInvalidBounds(t *testing.T) {
	env := testhelper.Setup(t)
	at := time.Date(2026, 11, 8, 4, 0, 0, 0, time.UTC)
	for _, query := range []events.JournalQuery{
		{Limit: -1}, {Limit: events.MaxJournalLimit + 1},
		{From: at, Before: at}, {From: at.Add(time.Minute), Before: at},
	} {
		_, err := env.Bus.InstanceJournal(t.Context(), query)
		require.Error(t, err)
	}
}
