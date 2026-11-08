package events_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/infra/events"
)

func TestSourceIdentitySeparatesSemanticFactsAndIsStableAcrossRedelivery(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		upstream := rapid.StringMatching(`[a-zA-Z0-9:._/@-]{1,80}`).Draw(t, "upstream")
		first := events.Envelope{ID: "first-envelope", Name: events.NameEmailDelivered, DedupKey: upstream}
		retry := first
		retry.ID = "retry-envelope"
		anotherFact := first
		anotherFact.Name = events.NameEmailBounced
		assert.Equal(t, first.SourceID(), retry.SourceID())
		assert.NotEqual(t, first.SourceID(), anotherFact.SourceID())
		assert.Equal(t, "unidentified-envelope", (events.Envelope{ID: "unidentified-envelope"}).SourceID())
	})
}
