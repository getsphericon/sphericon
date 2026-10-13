package operator_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/auditentry"
	enthold "github.com/getsphericon/sphericon/ent/hold"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var acme = serviceapi.EntityId(strconv.FormatInt(fixtures.AcmeID, 10))

func (h *harness) holdClient(t *testing.T) *serviceapi.Client {
	t.Helper()
	return h.env.ServiceClient(t, primary)
}

// setHold sets key on Acme expiring in d from the harness clock.
func (h *harness) setHold(t *testing.T, key string, d time.Duration) serviceapi.ServiceHoldsSetRes {
	t.Helper()
	return h.setHoldFor(t, key, "review", d)
}

// setHoldFor is setHold under a stated reason code.
func (h *harness) setHoldFor(t *testing.T, key, reasonCode string, d time.Duration) serviceapi.ServiceHoldsSetRes {
	t.Helper()
	res, err := h.holdClient(t).ServiceHoldsSet(t.Context(), &serviceapi.HoldInput{
		ReasonCode: reasonCode, Message: "Sending is paused for now.", ExpiresAt: serviceapi.Timestamp(h.clock.now().Add(d)),
	}, serviceapi.ServiceHoldsSetParams{WorkspaceId: acme, Key: key})
	require.NoError(t, err)
	return res
}

// setHoldResult is setHold for a call that is expected to succeed.
func (h *harness) setHoldResult(t *testing.T, key string, d time.Duration) *serviceapi.HoldSetResult {
	t.Helper()
	res, ok := h.setHold(t, key, d).(*serviceapi.HoldSetResult)
	require.Truef(t, ok, "got %T", res)
	return res
}

func (h *harness) clearHold(t *testing.T, key string) bool {
	t.Helper()
	res, err := h.holdClient(t).ServiceHoldsClear(t.Context(), serviceapi.ServiceHoldsClearParams{WorkspaceId: acme, Key: key})
	require.NoError(t, err)
	cleared, ok := res.(*serviceapi.HoldClearResult)
	require.Truef(t, ok, "got %T", res)
	return cleared.Changed
}

func (h *harness) listHolds(t *testing.T) []serviceapi.HoldResource {
	t.Helper()
	res, err := h.holdClient(t).ServiceHoldsList(t.Context(), serviceapi.ServiceHoldsListParams{WorkspaceId: acme})
	require.NoError(t, err)
	list, ok := res.(*serviceapi.HoldList)
	require.Truef(t, ok, "got %T", res)
	return list.Items
}

func (h *harness) requireHeld(t *testing.T, to string) {
	t.Helper()
	held, ok := h.sendOnAPI(t, to).(*externalapi.EmailsSendUnprocessableEntity)
	require.True(t, ok, "a send is refused with a 4xx")
	assert.Equal(t, outbound.HoldDetail(outbound.HoldOperator), held.Detail.Value)
}

func TestAHoldFreezesSendingAndClearingItResumes(t *testing.T) {
	h := newHarness(t)

	set := h.setHoldResult(t, "k1", time.Hour).Hold
	assert.Equal(t, "k1", set.Key)
	assert.Equal(t, "review", set.ReasonCode)
	assert.Equal(t, "Sending is paused for now.", set.Message)
	assert.Equal(t, fixtures.ServiceCredentialPrimaryName, set.SetBy)
	require.Len(t, h.listHolds(t), 1)

	h.requireHeld(t, "held@example.com")
	assert.Empty(t, h.env.CustomerMail.Messages(), "a held message goes nowhere")

	h.clearHold(t, "k1")
	assert.Empty(t, h.listHolds(t))
	assert.IsType(t, &externalapi.SendTransactionalEmailResponse{}, h.sendOnAPI(t, "after@example.com"), "sending resumes")
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

	assert.IsType(t, &externalapi.SendTransactionalEmailResponse{}, h.sendOnAPI(t, "lapsed@example.com"), "sending resumes at the expiry")
	assert.Empty(t, h.listHolds(t), "a lapsed hold is not listed")
	assert.Equal(t, 1, h.env.DB.Hold.Query().Where(enthold.Key("k1")).CountX(t.Context()), "the row stays: nothing depends on a sweep")
}

func TestASecondHoldKeepsSendingFrozenAfterTheFirstIsCleared(t *testing.T) {
	h := newHarness(t)
	h.setHold(t, "first", time.Hour)
	h.setHold(t, "second", time.Hour)

	h.clearHold(t, "first")
	h.requireHeld(t, "still@example.com")

	h.clearHold(t, "second")
	assert.IsType(t, &externalapi.SendTransactionalEmailResponse{}, h.sendOnAPI(t, "free@example.com"))
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
		res := h.setHold(t, "k", d)
		refused, ok := res.(*serviceapi.ServiceHoldsSetUnprocessableEntity)
		require.Truef(t, ok, "%s: got %T", name, res)
		assert.NotEmpty(t, refused.Detail.Value, name)
	}
	assert.Zero(t, h.env.DB.Hold.Query().Where(enthold.Key("k")).CountX(t.Context()), "a refused hold stores nothing")

	_, ok := h.setHold(t, "k", 30*24*time.Hour).(*serviceapi.HoldSetResult)
	assert.True(t, ok, "the maximum itself is accepted")
}

func TestTheMaximumExpiryIsConfigurable(t *testing.T) {
	h := newHarness(t, testhelper.WithConfig(func(c *config.Config) { c.OperatorHoldMaxExpiry = 2 * time.Hour }))

	assert.IsType(t, &serviceapi.ServiceHoldsSetUnprocessableEntity{}, h.setHold(t, "k", 3*time.Hour))
	assert.IsType(t, &serviceapi.HoldSetResult{}, h.setHold(t, "k", time.Hour))
}

func TestHoldsOfAnUnknownWorkspaceAreNotFound(t *testing.T) {
	h := newHarness(t)
	c := h.holdClient(t)

	list, err := c.ServiceHoldsList(t.Context(), serviceapi.ServiceHoldsListParams{WorkspaceId: "999999"})
	require.NoError(t, err)
	assert.IsType(t, &serviceapi.ServiceHoldsListNotFound{}, list)
	set, err := c.ServiceHoldsSet(t.Context(), &serviceapi.HoldInput{
		ReasonCode: "r", Message: "m", ExpiresAt: serviceapi.Timestamp(h.clock.now().Add(time.Hour)),
	}, serviceapi.ServiceHoldsSetParams{WorkspaceId: "999999", Key: "k"})
	require.NoError(t, err)
	assert.IsType(t, &serviceapi.ServiceHoldsSetNotFound{}, set)
	clear, err := c.ServiceHoldsClear(t.Context(), serviceapi.ServiceHoldsClearParams{WorkspaceId: "999999", Key: "k"})
	require.NoError(t, err)
	assert.IsType(t, &serviceapi.ServiceHoldsClearNotFound{}, clear)
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
	h.env.DeliverToEE(t)

	entries := h.env.DB.Scoped(fixtures.AcmeID).AuditEntry().Query().
		Where(auditentry.TargetName("audited")).Order(auditentry.ByID()).AllX(t.Context())

	var actions []string
	for _, e := range entries {
		actions = append(actions, e.Action)
		assert.Equal(t, fixtures.ServiceCredentialPrimaryName, *e.ActorName)
	}
	assert.Equal(t, []string{"hold.create", "hold.update", "hold.delete"}, actions)
}

func TestWithoutALicenseHoldsFreezeNothing(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	_, err := env.DB.Scoped(fixtures.AcmeID).Hold().Create().
		SetKey("stray").SetReasonCode("r").SetMessage("m").SetSetBy("x").SetExpiresAt(time.Now().Add(time.Hour)).Save(t.Context())
	require.NoError(t, err)
	h := &harness{env: env}

	assert.IsType(t, &externalapi.SendTransactionalEmailResponse{}, h.sendOnAPI(t, "free@example.com"))
}
