package events

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
)

// An Envelope travels as a watermill message the way watermill intends one: the payload is the
// event itself (Envelope.Data), and the rest of the envelope (name, version, Workspace, times,
// dedup key, cause) is the message's metadata, which a middleware can read without decoding
// the body. The message id is the envelope's ID.

// The metadata keys of an envelope.
const (
	MetaName        = "name"
	MetaVersion     = "version"
	MetaWorkspaceID = "workspace_id"
	MetaOccurredAt  = "occurred_at"
	MetaRecordedAt  = "recorded_at"
	MetaDedupKey    = "dedup_key"
	MetaCausedBy    = "caused_by"
)

// Message is the watermill message that carries env.
func Message(env Envelope) *message.Message {
	msg := message.NewMessage(env.ID, message.Payload(env.Data))
	msg.Metadata.Set(MetaName, env.Name)
	msg.Metadata.Set(MetaVersion, strconv.Itoa(env.Version))
	msg.Metadata.Set(MetaWorkspaceID, strconv.FormatInt(env.WorkspaceID, 10))
	msg.Metadata.Set(MetaOccurredAt, env.OccurredAt.Format(time.RFC3339Nano))
	msg.Metadata.Set(MetaRecordedAt, env.RecordedAt.Format(time.RFC3339Nano))
	if env.DedupKey != "" {
		msg.Metadata.Set(MetaDedupKey, env.DedupKey)
	}
	if env.CausedBy != "" {
		msg.Metadata.Set(MetaCausedBy, env.CausedBy)
	}
	if env.CorrelationID != "" {
		msg.Metadata.Set(MetaCorrelationID, env.CorrelationID)
	}
	if env.CausationID != "" {
		msg.Metadata.Set(MetaCausationID, env.CausationID)
	}
	return msg
}

// EnvelopeOf is the envelope msg carries.
func EnvelopeOf(msg *message.Message) (Envelope, error) {
	return EnvelopeFrom(msg.UUID, msg.Payload, msg.Metadata)
}

// EnvelopeFrom is the envelope of a message by its parts: its id, its payload and its metadata,
// as an outbox row stores them. A message published before the envelope moved to the metadata
// has none and carries the whole envelope as its payload; it is read as it was written, so the
// rows still in the outbox at an upgrade (and those an instance of the previous version
// publishes during a rolling one) are delivered too.
func EnvelopeFrom(id string, payload []byte, metadata map[string]string) (Envelope, error) {
	if metadata[MetaName] == "" {
		var env Envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return Envelope{}, fmt.Errorf("unmarshal envelope: %w", err)
		}
		return env, nil
	}
	version, err := strconv.Atoi(metadata[MetaVersion])
	if err != nil {
		return Envelope{}, fmt.Errorf("envelope %s version: %w", id, err)
	}
	workspaceID, err := strconv.ParseInt(metadata[MetaWorkspaceID], 10, 64)
	if err != nil {
		return Envelope{}, fmt.Errorf("envelope %s workspace: %w", id, err)
	}
	occurred, err := time.Parse(time.RFC3339Nano, metadata[MetaOccurredAt])
	if err != nil {
		return Envelope{}, fmt.Errorf("envelope %s occurred at: %w", id, err)
	}
	recorded, err := time.Parse(time.RFC3339Nano, metadata[MetaRecordedAt])
	if err != nil {
		return Envelope{}, fmt.Errorf("envelope %s recorded at: %w", id, err)
	}
	return Envelope{
		ID:          id,
		Name:        metadata[MetaName],
		Version:     version,
		WorkspaceID: workspaceID,
		OccurredAt:  occurred,
		RecordedAt:  recorded,
		Data:        payload,
		DedupKey:    metadata[MetaDedupKey],
		CausedBy:    metadata[MetaCausedBy],
		// the chain's ids (correlation.go)
		CorrelationID: metadata[MetaCorrelationID],
		CausationID:   metadata[MetaCausationID],
	}, nil
}
