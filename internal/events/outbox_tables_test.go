package events

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ent/outboxmessage"
	"github.com/getsphericon/sphericon/ent/outboxoffset"
)

// The ent clients for the outbox (ent/schema/outbox_message.go, outbox_offset.go) address
// the tables watermill creates, whose names the adapters derive from the topic.
func TestEntOutboxSchemasNameTheTablesWatermillCreates(t *testing.T) {
	assert.Equal(t, outboxTable(), strconv.Quote(outboxmessage.Table))
	assert.Equal(t, offsetsTable(), strconv.Quote(outboxoffset.Table))
}
