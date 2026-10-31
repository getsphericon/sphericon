package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/events"
)

// An envelope travels as its event in the payload and the rest in the metadata, and reads back
// whole.
func TestAnEnvelopeTravelsAsMetadataAndItsEvent(t *testing.T) {
	at := time.Date(2026, 10, 31, 12, 0, 0, 123, time.UTC)
	env := events.Envelope{
		ID: "01JEVENT", Name: events.NameContactCreated, Version: 2, WorkspaceID: 42,
		OccurredAt: at, RecordedAt: at.Add(time.Second), Data: json.RawMessage(`{"email":"ada@example.com"}`),
		DedupKey: "provider-1", CausedBy: "rule:7",
	}

	msg := events.Message(env)
	read, err := events.EnvelopeOf(msg)

	require.NoError(t, err)
	assert.JSONEq(t, `{"email":"ada@example.com"}`, string(msg.Payload), "the payload is the event itself")
	assert.Equal(t, events.NameContactCreated, msg.Metadata.Get(events.MetaName))
	assert.Equal(t, "42", msg.Metadata.Get(events.MetaWorkspaceID))
	assert.Equal(t, env, read)
}

// A message published before the envelope moved to the metadata carries it whole as its payload,
// and is read as it was written.
func TestAMessageOfThePreviousShapeIsReadAsItWasWritten(t *testing.T) {
	env := events.Envelope{ID: "01JOLD", Name: events.NameContactCreated, Version: 1, WorkspaceID: 7, Data: json.RawMessage(`{"email":"old@example.com"}`)}
	body, err := json.Marshal(env)
	require.NoError(t, err)

	read, err := events.EnvelopeOf(message.NewMessage("ignored", body))

	require.NoError(t, err)
	assert.Equal(t, "01JOLD", read.ID)
	assert.EqualValues(t, 7, read.WorkspaceID)
	assert.JSONEq(t, `{"email":"old@example.com"}`, string(read.Data))
}
