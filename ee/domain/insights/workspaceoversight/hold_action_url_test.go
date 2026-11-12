package workspaceoversight_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serviceapi "github.com/getsphericon/sphericon/gen/service"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const resolveURL = "https://account.sphericon.test/acme/resolve"

// trySetHoldWithURL sets a hold that carries an action URL.
func (h *harness) trySetHoldWithURL(t *testing.T, key, actionURL string) (*serviceapi.HoldSetResult, error) {
	t.Helper()
	return h.holdClient(t).ServiceHoldsSet(t.Context(), &serviceapi.HoldInput{
		ReasonCode: "review", Message: "Sending is paused for now.", ActionUrl: serviceapi.NewOptNilString(actionURL),
		ExpiresAt: serviceapi.Timestamp(h.clock.now().Add(time.Hour)),
	}, serviceapi.ServiceHoldsSetParams{WorkspaceId: acme, Key: key})
}

// setHoldWithURL is trySetHoldWithURL for a call that is expected to succeed.
func (h *harness) setHoldWithURL(t *testing.T, key, actionURL string) *serviceapi.HoldSetResult {
	t.Helper()
	res, err := h.trySetHoldWithURL(t, key, actionURL)
	require.NoError(t, err)
	return res
}

// A hold coexists with a suspension and lifts on its own: lifting it never lifts the suspension.
func TestLiftingAHoldLeavesTheSuspensionInForce(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	h.suspend(t, c, fixtures.AcmePublicID, "abuse report")
	h.env.ExtendHold(t.Context(), fixtures.AcmeID, fixtures.HoldAcmeLapsedID, h.clock.now().Add(time.Hour))

	assert.True(t, h.clearHold(t, fixtures.HoldAcmeLapsedKey))

	held := h.refusedSendOnAPI(t, "still-held@example.com", "the suspension still refuses sends")
	assert.Equal(t, "the message was not sent: "+outboundmessages.HoldDetail(holdreason.WorkspaceSuspended), held.Detail.Value)
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
	acme, found := lo.Find(list, func(w siteapi.SiteWorkspaceResource) bool { return w.ID == fixtures.AcmePublicID })
	require.True(t, found)
	require.Len(t, acme.Holds, 2)
	assert.Equal(t, resolveURL, acme.Holds[0].ActionUrl.Value)
	assert.False(t, acme.Holds[1].ActionUrl.Set)

	h.setHold(t, "a-with-link", time.Hour)
	assert.False(t, h.acmeHolds(t, c)[0].ActionUrl.Set, "setting the hold again without a URL removes the stored one")
}

func TestSettingAHoldWithItsActionURLAgainChangesNothing(t *testing.T) {
	h := newHarness(t)
	h.setHoldWithURL(t, "same", resolveURL)
	again := h.setHoldWithURL(t, "same", resolveURL)
	assert.False(t, again.Changed)
}

func TestAnActionURLMustBeAnAbsoluteHTTPSURLWithoutCredentials(t *testing.T) {
	h := newHarness(t)
	for _, bad := range []string{"https://user:pass@account.sphericon.test/", "https:///no-host"} {
		_, err := h.trySetHoldWithURL(t, "bad", bad)
		testhelper.ServiceRefusal(t, err, http.StatusUnprocessableEntity, "%q is refused", bad)
	}
	assert.Empty(t, h.listHolds(t))
}
