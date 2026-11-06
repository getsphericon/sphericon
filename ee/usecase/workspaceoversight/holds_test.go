package workspaceoversight_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/auditentry"
	enthold "github.com/getsphericon/sphericon/ent/hold"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var acme = serviceapi.WorkspaceId(fixtures.AcmePublicID)

func (h *harness) holdClient(t *testing.T) *serviceapi.Client {
	t.Helper()
	return h.env.ServiceClient(t, primary)
}

// trySetHold sets key on Acme expiring in d from the harness clock.
func (h *harness) trySetHold(t *testing.T, key string, d time.Duration) (*serviceapi.HoldSetResult, error) {
	t.Helper()
	return h.trySetHoldFor(t, key, "review", d)
}

// trySetHoldFor is trySetHold under a stated reason code.
func (h *harness) trySetHoldFor(t *testing.T, key, reasonCode string, d time.Duration) (*serviceapi.HoldSetResult, error) {
	t.Helper()
	return h.holdClient(t).ServiceHoldsSet(t.Context(), &serviceapi.HoldInput{
		ReasonCode: reasonCode, Message: "Sending is paused for now.", ExpiresAt: serviceapi.Timestamp(h.clock.now().Add(d)),
	}, serviceapi.ServiceHoldsSetParams{WorkspaceId: acme, Key: key})
}

// setHold is trySetHold for a call that is expected to succeed.
func (h *harness) setHold(t *testing.T, key string, d time.Duration) *serviceapi.HoldSetResult {
	t.Helper()
	res, err := h.trySetHold(t, key, d)
	require.NoError(t, err)
	return res
}

func (h *harness) clearHold(t *testing.T, key string) bool {
	t.Helper()
	cleared, err := h.holdClient(t).ServiceHoldsClear(t.Context(), serviceapi.ServiceHoldsClearParams{WorkspaceId: acme, Key: key})
	require.NoError(t, err)
	return cleared.Changed
}

func (h *harness) listHolds(t *testing.T) []serviceapi.HoldResource {
	t.Helper()
	list, err := h.holdClient(t).ServiceHoldsList(t.Context(), serviceapi.ServiceHoldsListParams{WorkspaceId: acme})
	require.NoError(t, err)
	return list.Items
}

func (h *harness) requireHeld(t *testing.T, to string) {
	t.Helper()
	held := h.refusedSendOnAPI(t, to, "a send is refused with a 4xx")
	assert.Equal(t, "the message was not sent: "+outboundmessages.Detail(holds.ReasonStaff), held.Detail.Value)
}

func TestAHoldFreezesSendingAndClearingItResumes(t *testing.T) {
	h := newHarness(t)

	set := h.setHold(t, "k1", time.Hour).Hold
	assert.Equal(t, "k1", set.Key)
	assert.Equal(t, "review", set.ReasonCode)
	assert.Equal(t, "Sending is paused for now.", set.Message)
	assert.Equal(t, fixtures.ServiceCredentialPrimaryName, set.SetBy)
	require.Len(t, h.listHolds(t), 1)

	h.requireHeld(t, "held@example.com")
	assert.Empty(t, h.env.CustomerMail.Messages(), "a held message goes nowhere")

	h.clearHold(t, "k1")
	assert.Empty(t, h.listHolds(t))
	h.sendOnAPI(t, "after@example.com")
}

func TestSettingTheSameHoldTwiceIsHarmlessAndRenewingMovesItsExpiry(t *testing.T) {
	h := newHarness(t)
	h.setHold(t, "k1", time.Hour)
	h.setHold(t, "k1", time.Hour)
	require.Len(t, h.listHolds(t), 1, "the key is unique per Workspace")

	h.setHold(t, "k1", 3*time.Hour)
	h.clock.t = h.clock.t.Add(2 * time.Hour)
	h.requireHeld(t, "renewed@example.com")
	items := h.listHolds(t)
	require.Len(t, items, 1)
	assert.WithinDuration(t, h.clock.now().Add(time.Hour), time.Time(items[0].ExpiresAt), time.Second)
}

func TestAHoldPastItsExpiryIsIgnoredAtOnceWithNoCleanup(t *testing.T) {
	h := newHarness(t)
	h.setHold(t, "k1", time.Hour)
	h.requireHeld(t, "before@example.com")

	h.clock.t = h.clock.t.Add(time.Hour + time.Second)

	h.sendOnAPI(t, "lapsed@example.com")
	assert.Empty(t, h.listHolds(t), "a lapsed hold is not listed")
	assert.Equal(t, 1, h.env.DB.Hold.Query().Where(enthold.Key("k1")).CountX(t.Context()), "the row stays: nothing depends on a sweep")
}

func TestASecondHoldKeepsSendingFrozenAfterTheFirstIsCleared(t *testing.T) {
	h := newHarness(t)
	h.env.ExtendHold(t.Context(), fixtures.AcmeID, fixtures.HoldAcmeLapsedID, h.clock.now().Add(time.Hour))
	h.env.ExtendHold(t.Context(), fixtures.AcmeID, fixtures.HoldAcmeSecondLapsedID, h.clock.now().Add(time.Hour))

	h.clearHold(t, fixtures.HoldAcmeLapsedKey)
	h.requireHeld(t, "still@example.com")

	h.clearHold(t, fixtures.HoldAcmeSecondLapsedKey)
	h.sendOnAPI(t, "free@example.com")
}

func TestClearingAMissingHoldSucceeds(t *testing.T) {
	h := newHarness(t)

	h.clearHold(t, "never-set")
	h.clearHold(t, "never-set")
}

func TestAHoldNeedsAFutureExpiryWithinTheMaximum(t *testing.T) {
	h := newHarness(t)

	for name, d := range map[string]time.Duration{
		"already past":    -time.Minute,
		"beyond the max":  30*24*time.Hour + time.Hour,
		"far beyond":      5 * 365 * 24 * time.Hour,
		"exactly lapsing": 0,
	} {
		_, err := h.trySetHold(t, "k", d)
		refused := testhelper.ServiceRefusal(t, err, http.StatusUnprocessableEntity, name)
		assert.NotEmpty(t, refused.Detail.Value, name)
	}
	assert.Zero(t, h.env.DB.Hold.Query().Where(enthold.Key("k")).CountX(t.Context()), "a refused hold stores nothing")

	_, err := h.trySetHold(t, "k", 30*24*time.Hour)
	assert.NoError(t, err, "the maximum itself is accepted")
}

func TestTheMaximumExpiryIsConfigurable(t *testing.T) {
	h := newHarness(t, testhelper.WithConfig(func(c *config.Config) { c.StaffHoldMaxExpiry = 2 * time.Hour }))

	_, err := h.trySetHold(t, "k", 3*time.Hour)
	testhelper.ServiceRefusal(t, err, http.StatusUnprocessableEntity)
	_, err = h.trySetHold(t, "k", time.Hour)
	assert.NoError(t, err)
}

// Workspace deletion (ADR 0043) is a Hold source of its own: the holds readers report it
// beside the Staff holds while the Workspace is deleted, and restoring lifts it alone.
func TestAHoldListReportsThePendingDeletionOfADeletedWorkspace(t *testing.T) {
	h := newHarness(t)
	h.env.ExtendHold(t.Context(), fixtures.AcmeID, fixtures.HoldAcmeLapsedID, h.clock.now().Add(time.Hour))
	discardedAt := h.clock.now().Add(-time.Hour).UTC().Truncate(time.Second)
	h.env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(discardedAt).ExecX(t.Context())

	items := h.listHolds(t)
	require.Len(t, items, 2)
	assert.Equal(t, holds.KeyPendingDeletion, items[0].Key)
	assert.Equal(t, holds.ReasonPendingDeletion, items[0].ReasonCode)
	assert.Equal(t, fixtures.HoldAcmeLapsedKey, items[1].Key)

	h.env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).ClearDiscardedAt().ExecX(t.Context())
	items = h.listHolds(t)
	require.Len(t, items, 1)
	assert.Equal(t, fixtures.HoldAcmeLapsedKey, items[0].Key)
}

func TestHoldsOfAnUnknownWorkspaceAreNotFound(t *testing.T) {
	h := newHarness(t)
	c := h.holdClient(t)

	_, err := c.ServiceHoldsList(t.Context(), serviceapi.ServiceHoldsListParams{WorkspaceId: "NoSuchId"})
	testhelper.ServiceRefusal(t, err, http.StatusNotFound)
	_, err = c.ServiceHoldsSet(t.Context(), &serviceapi.HoldInput{
		ReasonCode: "r", Message: "m", ExpiresAt: serviceapi.Timestamp(h.clock.now().Add(time.Hour)),
	}, serviceapi.ServiceHoldsSetParams{WorkspaceId: "NoSuchId", Key: "k"})
	testhelper.ServiceRefusal(t, err, http.StatusNotFound)
	_, err = c.ServiceHoldsClear(t.Context(), serviceapi.ServiceHoldsClearParams{WorkspaceId: "NoSuchId", Key: "k"})
	testhelper.ServiceRefusal(t, err, http.StatusNotFound)
}

func TestHoldsNeedTheWriteHoldsScope(t *testing.T) {
	h := newHarness(t)

	_, err := h.env.ServiceClient(t, readOnly).ServiceHoldsList(t.Context(), serviceapi.ServiceHoldsListParams{WorkspaceId: acme})
	require.Error(t, err, "a credential without the scope is refused")
	_, err = h.env.ServiceClient(t, readOnly).ServiceHoldsSet(t.Context(), &serviceapi.HoldInput{
		ReasonCode: "r", Message: "m", ExpiresAt: serviceapi.Timestamp(h.clock.now().Add(time.Hour)),
	}, serviceapi.ServiceHoldsSetParams{WorkspaceId: acme, Key: "k"})
	require.Error(t, err)
	assert.Zero(t, h.env.DB.Hold.Query().Where(enthold.Key("k")).CountX(t.Context()))
}

func TestHoldEndpointsAreAbsentFromThePublicPort(t *testing.T) {
	h := newHarness(t)

	_, err := h.env.ServiceClientOnPublicPort(t, primary).ServiceHoldsList(t.Context(), serviceapi.ServiceHoldsListParams{WorkspaceId: acme})

	require.Error(t, err)
}

func TestEveryChangingSetRenewalAndClearIsRecordedUnderTheCredentialsName(t *testing.T) {
	h := newHarness(t)
	h.setHold(t, "audited", time.Hour)
	h.setHold(t, "audited", time.Hour)
	h.setHold(t, "audited", 2*time.Hour)
	h.clearHold(t, "audited")
	h.clearHold(t, "audited")
	h.env.WorkEditionJobs(t)

	entries := h.env.DB.Scoped(fixtures.AcmeID).AuditEntry().Query().
		Where(auditentry.TargetName("audited")).Order(auditentry.ByID()).AllX(t.Context())

	actions := make([]string, 0, len(entries))
	for _, e := range entries {
		actions = append(actions, e.Action)
		assert.Equal(t, fixtures.ServiceCredentialPrimaryName, *e.ActorName)
	}
	assert.Equal(t, []string{"hold.create", "hold.update", "hold.delete"}, actions)
}

func TestWithoutALicenseHoldsFreezeNothing(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	env.ExtendHold(t.Context(), fixtures.AcmeID, fixtures.HoldAcmeLapsedID, time.Now().Add(time.Hour))
	h := &harness{env: env}

	h.sendOnAPI(t, "free@example.com")
}
