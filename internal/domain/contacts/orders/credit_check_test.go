package orders_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The database refuses an Order credited to both a Broadcast and an Automation, or credited without
// the click time and the clicked message, or with either and no credit, whatever code writes it
// (orders_credit_check), and a currency that is no ISO 4217 code in capitals
// (orders_currency_format).
func TestTheDatabaseRefusesABrokenCredit(t *testing.T) {
	cases := map[string]func(c *ent.OrderScopedCreate){
		"both credited": func(c *ent.OrderScopedCreate) {
			c.SetBroadcastID(fixtures.BroadcastSentID).SetAutomationID(fixtures.AutomationWelcomeSeriesID).
				SetCreditedClickAt(time.Now()).SetCreditedMessageID(fixtures.OutboundMessageTrackingSampleID)
		},
		"credited without a click": func(c *ent.OrderScopedCreate) {
			c.SetBroadcastID(fixtures.BroadcastSentID).SetCreditedMessageID(fixtures.OutboundMessageTrackingSampleID)
		},
		"credited without a message": func(c *ent.OrderScopedCreate) {
			c.SetBroadcastID(fixtures.BroadcastSentID).SetCreditedClickAt(time.Now())
		},
		"a click without a credit": func(c *ent.OrderScopedCreate) {
			c.SetCreditedClickAt(time.Now()).SetCreditedMessageID(fixtures.OutboundMessageTrackingSampleID)
		},
		"a message without a credit": func(c *ent.OrderScopedCreate) {
			c.SetCreditedMessageID(fixtures.OutboundMessageTrackingSampleID)
		},
		"a lower-case currency": func(c *ent.OrderScopedCreate) { c.SetCurrency("usd") },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			// test writes: the refused insert is the operation under test.
			create := env.DB.Scoped(fixtures.AcmeID).Order().Create().SetOrderID("broken").SetCurrency("USD")
			change(create)
			err := create.Exec(t.Context())
			require.Error(t, err)
			assert.True(t, ent.IsConstraintError(err), "a constraint refuses it: %v", err)
		})
	}
}
