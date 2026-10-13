package operator_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	operatorapi "github.com/getsphericon/sphericon/gen/operator"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
)

// A hold whose reason code is non_payment or free_plan_cap is a billing hold. It
// coexists with a suspension and lifts on its own: lifting it never lifts the suspension.
func TestLiftingABillingHoldLeavesTheSuspensionInForce(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)
	h.suspend(t, c, fixtures.AcmeID, "abuse report")
	h.setHoldFor(t, "billing", "non_payment", time.Hour)

	assert.True(t, h.clearHold(t, "billing"))

	held, ok := h.sendOnAPI(t, "still-held@example.com").(*externalapi.EmailsSendUnprocessableEntity)
	require.True(t, ok, "the suspension still refuses sends")
	assert.Equal(t, outbound.HoldDetail(outbound.HoldSuspended), held.Detail.Value)
	assert.NotNil(t, h.env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).SuspendedAt)
}

func TestTheConsoleAndTheOwnerTellABillingHoldFromAnotherHold(t *testing.T) {
	h := newHarness(t)
	h.setHoldFor(t, "a-billing", "non_payment", time.Hour)
	h.setHoldFor(t, "b-cap", "free_plan_cap", time.Hour)
	h.setHoldFor(t, "c-review", "review", time.Hour)
	c, _ := h.operatorSession(t)

	console := h.acmeHolds(t, c)
	require.Len(t, console, 3)
	assert.Equal(t, []bool{true, true, false}, []bool{console[0].Billing, console[1].Billing, console[2].Billing})

	list, err := h.env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	var checked bool
	for _, w := range list {
		if w.Slug != fixtures.AcmeSlug {
			continue
		}
		checked = true
		require.Len(t, w.Holds, 3)
		assert.Equal(t, []bool{true, true, false}, []bool{w.Holds[0].Billing, w.Holds[1].Billing, w.Holds[2].Billing})
	}
	assert.True(t, checked)
}

// The Audit log shows a billing hold's set and lift as billing activity: the console names
// the credential, the customer reads "sphericon staff", never an individual.
func TestABillingHoldInTheAuditLogIsBillingActivityUnderTheCredential(t *testing.T) {
	h := newHarness(t)
	h.setHoldFor(t, "billing", "non_payment", time.Hour)
	h.setHoldFor(t, "other", "review", time.Hour)
	h.clearHold(t, "billing")
	h.env.DeliverToEE(t)

	c, _ := h.operatorSession(t)
	var consoleBilling, consoleOther []operatorapi.OperatorAuditEntryResource
	for _, e := range h.auditPage(t, c, fixtures.AcmeID, operatorapi.OperatorWorkspaceAuditListParams{}).Items {
		switch e.Target.Name.Value {
		case "billing":
			consoleBilling = append(consoleBilling, e)
		case "other":
			consoleOther = append(consoleOther, e)
		}
	}
	require.Len(t, consoleBilling, 2, "set and lift")
	for _, e := range consoleBilling {
		assert.True(t, e.Billing)
		assert.Equal(t, fixtures.ServiceCredentialPrimaryName, e.Actor.Name.Value)
	}
	require.Len(t, consoleOther, 1)
	assert.False(t, consoleOther[0].Billing)

	res, err := h.env.SiteActor(t, fixtures.OwnerJohnEmail).SiteAuditList(t.Context(), siteapi.SiteAuditListParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	var seen int
	for _, e := range res.(*siteapi.SiteAuditEntryList).Items {
		switch e.Target.Name.Value {
		case "billing":
			seen++
			assert.True(t, e.Billing)
			assert.Equal(t, siteapi.SiteAuditActorKindOperator, e.Actor.Kind)
		case "other":
			assert.False(t, e.Billing)
		}
	}
	assert.Equal(t, 2, seen)
}
