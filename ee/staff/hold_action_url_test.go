package staff_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
)

const resolveURL = "https://account.sphericon.test/acme/resolve"

// setHoldWithURL sets a hold that carries an action URL.
func (h *harness) setHoldWithURL(t *testing.T, key, actionURL string) serviceapi.ServiceHoldsSetRes {
	t.Helper()
	res, err := h.holdClient(t).ServiceHoldsSet(t.Context(), &serviceapi.HoldInput{
		ReasonCode: "review", Message: "Sending is paused for now.", ActionUrl: serviceapi.NewOptNilString(actionURL),
		ExpiresAt: serviceapi.Timestamp(h.clock.now().Add(time.Hour)),
	}, serviceapi.ServiceHoldsSetParams{WorkspaceId: acme, Key: key})
	require.NoError(t, err)
	return res
}

// A hold coexists with a suspension and lifts on its own: lifting it never lifts the suspension.
func TestLiftingAHoldLeavesTheSuspensionInForce(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	h.suspend(t, c, fixtures.AcmePublicID, "abuse report")
	h.setHold(t, "review", time.Hour)

	assert.True(t, h.clearHold(t, "review"))

	held, ok := h.sendOnAPI(t, "still-held@example.com").(*externalapi.EmailsSendUnprocessableEntity)
	require.True(t, ok, "the suspension still refuses sends")
	assert.Equal(t, outbound.Detail(outbound.HoldSuspended), held.Detail.Value)
	assert.NotNil(t, h.env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).SuspendedAt)
}

func TestTheActionURLReachesTheConsoleAndTheOwnerAndRenewingWithoutItRemovesIt(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	h.setHoldWithURL(t, "a-with-link", resolveURL)
	h.setHold(t, "b-without-link", time.Hour)

	console := h.acmeHolds(t, c)
	require.Len(t, console, 2)
	assert.Equal(t, resolveURL, console[0].ActionUrl.Value)
	assert.False(t, console[1].ActionUrl.Set)

	list, err := h.env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	var checked bool
	for _, w := range list {
		if w.ID != fixtures.AcmePublicID {
			continue
		}
		checked = true
		require.Len(t, w.Holds, 2)
		assert.Equal(t, resolveURL, w.Holds[0].ActionUrl.Value)
		assert.False(t, w.Holds[1].ActionUrl.Set)
	}
	assert.True(t, checked)

	h.setHold(t, "a-with-link", time.Hour)
	assert.False(t, h.acmeHolds(t, c)[0].ActionUrl.Set, "setting the hold again without a URL removes the stored one")
}

func TestSettingAHoldWithItsActionURLAgainChangesNothing(t *testing.T) {
	h := newHarness(t)
	h.setHoldWithURL(t, "same", resolveURL)
	again, ok := h.setHoldWithURL(t, "same", resolveURL).(*serviceapi.HoldSetResult)
	require.True(t, ok)
	assert.False(t, again.Changed)
}

func TestAnActionURLMustBeAnAbsoluteHTTPSURLWithoutCredentials(t *testing.T) {
	h := newHarness(t)
	for _, bad := range []string{"https://user:pass@account.sphericon.test/", "https:///no-host"} {
		_, ok := h.setHoldWithURL(t, "bad", bad).(*serviceapi.ServiceHoldsSetUnprocessableEntity)
		assert.Truef(t, ok, "%q is refused", bad)
	}
	assert.Empty(t, h.listHolds(t))
}
