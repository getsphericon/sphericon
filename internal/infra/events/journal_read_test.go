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
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestPublicationCanBeInspectedWithoutPayloadOrNewWork(t *testing.T) {
	at := time.Date(2026, 11, 8, 4, 0, 0, 0, time.UTC)
	observed := at.Add(-time.Minute)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	err := env.Bus.Publish(t.Context(), &events.EmailEngagement{
		Action: systemevent.EmailOpened, WorkspaceID: fixtures.AcmeID,
		OutboundMessageID: fixtures.OutboundMessageTrackingSampleID, ContactID: fixtures.ContactAliceID,
		Email: fixtures.ContactAliceEmail, ObservedAt: observed,
	})
	require.NoError(t, err)
	kind := jobs.MessageObservationArgs{}.Kind()
	before := env.JobsOf(t, kind)
	page, err := env.Bus.WorkspaceJournal(t.Context(), fixtures.AcmeID, events.JournalQuery{Name: systemevent.EmailOpened})
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	entry := page.Entries[0]
	assert.NotEmpty(t, entry.EventID)
	assert.Equal(t, systemevent.EmailOpened, entry.Name)
	assert.Equal(t, 1, entry.Version)
	assert.WithinDuration(t, at, entry.RecordedAt, 0)
	assert.WithinDuration(t, observed, entry.OccurredAt, 0)
	assert.Equal(t, []string{kind}, entry.RetainedConsumers)
	assert.False(t, page.Truncated)
	after := env.JobsOf(t, kind)
	assert.Equal(t, before, after, "inspection queues no report work")
	encoded, err := json.Marshal(page)
	require.NoError(t, err)
	output := string(encoded)
	assert.NotContains(t, output, fixtures.ContactAliceEmail)
	assert.NotContains(t, output, "workspaceId")
	assert.NotContains(t, output, "outboundMessageId")
	assert.NotContains(t, output, "args")
}

func TestJournalInspectionKeepsWorkspaceAndInstanceHistorySeparate(t *testing.T) {
	env := testhelper.Setup(t)
	query := events.JournalQuery{Name: fixtures.JournalOldestName}
	globex, err := env.Bus.WorkspaceJournal(t.Context(), fixtures.GlobexID, query)
	require.NoError(t, err)
	other, err := env.Bus.WorkspaceJournal(t.Context(), fixtures.KramericaID, query)
	require.NoError(t, err)
	instance, err := env.Bus.InstanceJournal(t.Context(), events.JournalQuery{})
	require.NoError(t, err)
	require.Len(t, globex.Entries, 4)
	require.Len(t, other.Entries, 1)
	require.Len(t, instance.Entries, 1)
	assert.Equal(t, fixtures.JournalLatestTieEventID, globex.Entries[0].EventID)
	assert.Equal(t, fixtures.JournalOtherWorkspaceEventID, other.Entries[0].EventID)
	assert.Equal(t, fixtures.JournalInstanceEventID, instance.Entries[0].EventID)
	assert.Empty(t, instance.Entries[0].RetainedConsumers)
	missing, err := env.Bus.WorkspaceJournal(t.Context(), fixtures.KramericaID, events.JournalQuery{EventID: fixtures.JournalOldestEventID})
	require.NoError(t, err)
	assert.Empty(t, missing.Entries, "an event id cannot cross the selected scope")
}

func TestJournalInspectionCombinesExactFiltersWithRecordedTimeBounds(t *testing.T) {
	env := testhelper.Setup(t)
	globex := int64(fixtures.GlobexID)
	from := time.Date(2026, 11, 8, 4, 0, 0, 0, time.UTC)
	before := from.Add(2 * time.Minute)
	page, err := env.Bus.WorkspaceJournal(t.Context(), globex, events.JournalQuery{Name: fixtures.JournalOldestName, From: from, Before: before})
	require.NoError(t, err)
	require.Len(t, page.Entries, 2)
	assert.Equal(t, fixtures.JournalMiddleEventID, page.Entries[0].EventID)
	assert.Equal(t, fixtures.JournalOldestEventID, page.Entries[1].EventID)
	exact, err := env.Bus.WorkspaceJournal(t.Context(), globex, events.JournalQuery{EventID: fixtures.JournalMiddleEventID})
	require.NoError(t, err)
	assert.Equal(t, page.Entries[:1], exact.Entries)
	wrongName, err := env.Bus.WorkspaceJournal(t.Context(), globex, events.JournalQuery{Name: fixtures.JournalOtherTypeName, EventID: fixtures.JournalMiddleEventID})
	require.NoError(t, err)
	assert.Empty(t, wrongName.Entries, "filters combine rather than widening the selection")
	for _, bounds := range []events.JournalQuery{
		{Name: fixtures.JournalOldestName, Before: before},
		{Name: fixtures.JournalOldestName, From: from, Before: before},
	} {
		bounded, err := env.Bus.WorkspaceJournal(t.Context(), globex, bounds)
		require.NoError(t, err)
		assert.Equal(t, page.Entries, bounded.Entries)
	}
	later, err := env.Bus.WorkspaceJournal(t.Context(), globex, events.JournalQuery{Name: fixtures.JournalOldestName, From: before})
	require.NoError(t, err)
	require.Len(t, later.Entries, 2)
	assert.Equal(t, fixtures.JournalLatestTieEventID, later.Entries[0].EventID)
	assert.Equal(t, fixtures.JournalEarlierTieEventID, later.Entries[1].EventID)
}

func TestJournalInspectionBoundsOutputAndSortsRetainedConsumerNames(t *testing.T) {
	env := testhelper.Setup(t)
	globex := int64(fixtures.GlobexID)
	bounded, err := env.Bus.WorkspaceJournal(t.Context(), globex, events.JournalQuery{Name: fixtures.JournalOldestName, Limit: 1})
	require.NoError(t, err)
	require.Len(t, bounded.Entries, 1)
	assert.True(t, bounded.Truncated)
	entry := bounded.Entries[0]
	assert.Equal(t, fixtures.JournalLatestTieEventID, entry.EventID)
	assert.Equal(t, 2, entry.Version)
	assert.Equal(t, []string{"a_retired_report", "z_retired_report"}, entry.RetainedConsumers)
	for _, limit := range []int{0, 4, events.MaxJournalLimit} {
		full, err := env.Bus.WorkspaceJournal(t.Context(), globex, events.JournalQuery{Name: fixtures.JournalOldestName, Limit: limit})
		require.NoError(t, err)
		require.Len(t, full.Entries, 4)
		assert.False(t, full.Truncated)
		assert.Equal(t, entry, full.Entries[0])
		assert.Equal(t, fixtures.JournalEarlierTieEventID, full.Entries[1].EventID)
	}
}

func TestJournalInspectionRefusesInvalidBounds(t *testing.T) {
	env := testhelper.Setup(t)
	at := time.Date(2026, 11, 8, 4, 0, 0, 0, time.UTC)
	later := at.Add(time.Minute)
	for _, query := range []events.JournalQuery{
		{Limit: -1}, {Limit: events.MaxJournalLimit + 1},
		{From: at, Before: at}, {From: later, Before: at},
	} {
		_, err := env.Bus.InstanceJournal(t.Context(), query)
		require.Error(t, err)
	}
}
