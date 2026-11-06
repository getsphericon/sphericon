package ingesthook_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/usecase/ingesthook"
)

// A provider's event id is unique only per provider account (a Telegram update_id per bot), so
// the key a fact deduplicates on names the Channel and the Integration too.
func TestADedupKeyNamesTheChannelAndTheIntegration(t *testing.T) {
	tx := ingesthook.Tx{Integration: &ent.Integration{ID: 7, Channel: channels.SMS}}

	assert.Equal(t, "sms:7:1234", tx.DedupKey("1234"))
}
