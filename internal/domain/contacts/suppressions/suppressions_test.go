package suppressions_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestAddNormalizesAndSuppressesInEligibility(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)

	got, err := suppressions.AddManual(ctx, s, channels.Email, "  Blocked@Example.COM ")
	require.NoError(t, err)
	assert.Equal(t, "blocked@example.com", got.Destination)
	assert.Equal(t, entsuppression.ReasonManual, got.Reason)

	verdict, err := eligibility.Check(ctx, s, eligibility.ChannelEmail, "BLOCKED@example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.False(t, verdict.Eligible)
	assert.Equal(t, eligibility.ReasonSuppressed, verdict.Reason)
}

func TestAddIsIdempotentAndKeepsTheFirstReason(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)

	got, err := suppressions.AddManual(ctx, s, channels.Email, fixtures.SuppressionGhostBounceDestination)
	require.NoError(t, err)
	assert.Equal(t, entsuppression.ReasonBounce, got.Reason)
	assert.Equal(t, int64(fixtures.SuppressionGhostBounceID), got.ID)
}

func TestAddRefusesABlankDestination(t *testing.T) {
	env := testhelper.Setup(t)
	err := suppressions.Add(t.Context(), env.DB.Scoped(fixtures.AcmeID), suppressions.Entry{Destination: "   ", Reason: entsuppression.ReasonManual})
	assert.ErrorIs(t, err, suppressions.ErrDestinationEmpty)
}

func TestAddIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	got, err := suppressions.AddManual(ctx, env.DB.Scoped(fixtures.GlobexID), channels.Email, fixtures.ContactAliceEmail)
	require.NoError(t, err)
	assert.Equal(t, int64(fixtures.GlobexID), got.WorkspaceID)

	leaked, err := env.DB.Scoped(fixtures.AcmeID).Suppression().Query().
		Where(entsuppression.DestinationEQ(fixtures.ContactAliceEmail)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, leaked)
}

func TestListPagesNewestFirst(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)

	all, err := suppressions.List(ctx, s, pagination.Params{Page: 1, PageSize: 100}, false)
	require.NoError(t, err)
	first, err := suppressions.List(ctx, s, pagination.Params{Page: 1, PageSize: 2}, false)
	require.NoError(t, err)
	assert.Len(t, first.Items, 2)
	assert.Equal(t, all.TotalItems, first.TotalItems)
	assert.Equal(t, all.Items[0].ID, first.Items[0].ID)
	assert.Greater(t, all.Items[0].ID, all.Items[1].ID)
}

func TestRemoveLiftsAndReportsAMissingOrLiftedRow(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)

	added, err := suppressions.AddManual(ctx, s, channels.Email, "gone@example.com")
	require.NoError(t, err)
	require.NoError(t, suppressions.Remove(ctx, s, added.ID, time.Now()))

	err = suppressions.Remove(ctx, s, added.ID, time.Now())
	require.ErrorIs(t, err, suppressions.ErrNotFound, "a lifted row cannot be lifted again")
	require.ErrorIs(t, suppressions.Remove(ctx, s, 987654321, time.Now()), suppressions.ErrNotFound)

	kept, err := s.Suppression().Get(ctx, added.ID)
	require.NoError(t, err)
	assert.NotNil(t, kept.LiftedAt)
}

func TestNormalizeDestinationTrimsAndLowerCases(t *testing.T) {
	assert.Equal(t, "a@x.com", suppressions.NormalizeDestination(" A@X.com "))
}

func TestNormalizeDestinationProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.String().Draw(t, "raw")
		once := suppressions.NormalizeDestination(raw)
		assert.Equal(t, once, suppressions.NormalizeDestination(once), "idempotent")
		assert.Equal(t, strings.ToLower(once), once, "no lower-case form left")
		assert.Equal(t, strings.TrimSpace(once), once, "trimmed")
	})
}

func TestForEventSuppressesAComplaintAndAHardBounceOnly(t *testing.T) {
	complaint, ok := suppressions.ForEvent(events.NameEmailComplained, "")
	assert.True(t, ok)
	assert.Equal(t, entsuppression.ReasonComplaint, complaint)

	hard, ok := suppressions.ForEvent(events.NameEmailBounced, events.BounceKindHard)
	assert.True(t, ok)
	assert.Equal(t, entsuppression.ReasonBounce, hard)

	_, soft := suppressions.ForEvent(events.NameEmailBounced, events.BounceKindSoft)
	assert.False(t, soft)
	_, other := suppressions.ForEvent("email.opened", "")
	assert.False(t, other)
}

// A typed number is suppressed on sms in E.164, and SMS Send-eligibility refuses it; text that is
// no phone number with its country code is refused on the destination.
func TestAManualSMSSuppressionStoresTheNumberInE164(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)

	got, err := suppressions.AddManual(ctx, s, channels.SMS, " +1 (415) 555-0132 ")
	require.NoError(t, err)
	assert.Equal(t, "+14155550132", got.Destination)
	assert.Equal(t, channels.SMS, got.Channel)

	verdict, err := eligibility.Check(ctx, s, eligibility.ChannelSMS, "+14155550132", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, eligibility.ReasonSuppressed, verdict.Reason)

	for _, typed := range []string{"4155550132", "not a number", "+1"} {
		_, err := suppressions.AddManual(ctx, s, channels.SMS, typed)
		assert.ErrorIs(t, err, suppressions.ErrDestinationInvalid, typed)
	}
}
