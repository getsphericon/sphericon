package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/events"
)

// An empty subscription list means "all customer-facing events": an Audit entry must
// be named explicitly, so an endpoint never leaks the Audit log by accident.
func TestEmptySubscriptionNeverMatchesAuditEntries(t *testing.T) {
	assert.True(t, matchesEvent(nil, events.NameContactCreated), "empty list still means all customer events")
	assert.False(t, matchesEvent(nil, events.NameAuditEntry))
	assert.False(t, matchesEvent([]string{}, events.NameAuditEntry))
	assert.True(t, matchesEvent([]string{events.NameAuditEntry}, events.NameAuditEntry), "explicit selection matches")
	assert.False(t, matchesEvent([]string{events.NameContactCreated}, events.NameAuditEntry))
}
