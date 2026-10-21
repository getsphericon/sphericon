package suppression_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/suppression"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestAddNormalizesAndSuppressesInEligibility(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)

	got, err := suppression.AddManual(ctx, s, "  Blocked@Example.COM ")
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

	got, err := suppression.AddManual(ctx, s, fixtures.SuppressionGhostBounceDestination)
	require.NoError(t, err)
	assert.Equal(t, entsuppression.ReasonBounce, got.Reason)
	assert.Equal(t, int64(fixtures.SuppressionGhostBounceID), int64(got.ID))
}

func TestAddRefusesABlankDestination(t *testing.T) {
	env := testhelper.Setup(t)
	err := suppression.Add(t.Context(), env.DB.Scoped(fixtures.AcmeID), suppression.Entry{Destination: "   ", Reason: entsuppression.ReasonManual})
	assert.ErrorIs(t, err, suppression.ErrDestinationEmpty)
}

func TestAddIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	got, err := suppression.AddManual(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.ContactAliceEmail)
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

	all, err := suppression.List(ctx, s, pagination.Params{Page: 1, PageSize: 100}, false)
	require.NoError(t, err)
	first, err := suppression.List(ctx, s, pagination.Params{Page: 1, PageSize: 2}, false)
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

	added, err := suppression.AddManual(ctx, s, "gone@example.com")
	require.NoError(t, err)
	require.NoError(t, suppression.Remove(ctx, s, added.ID))

	err = suppression.Remove(ctx, s, added.ID)
	require.ErrorIs(t, err, suppression.ErrNotFound, "a lifted row cannot be lifted again")
	require.ErrorIs(t, suppression.Remove(ctx, s, 987654321), suppression.ErrNotFound)

	kept, err := s.Suppression().Get(ctx, added.ID)
	require.NoError(t, err)
	assert.NotNil(t, kept.LiftedAt)
}

func TestNormalizeDestinationTrimsAndLowerCases(t *testing.T) {
	assert.Equal(t, "a@x.com", suppression.NormalizeDestination(" A@X.com "))
}

func TestNormalizeDestinationProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.String().Draw(t, "raw")
		once := suppression.NormalizeDestination(raw)
		assert.Equal(t, once, suppression.NormalizeDestination(once), "idempotent")
		assert.Equal(t, strings.ToLower(once), once, "no lower-case form left")
		assert.Equal(t, strings.TrimSpace(once), once, "trimmed")
	})
}

func TestForEventSuppressesAComplaintAndAPermanentBounceOnly(t *testing.T) {
	complaint, ok := suppression.ForEvent(suppression.EventComplained, "")
	assert.True(t, ok)
	assert.Equal(t, entsuppression.ReasonComplaint, complaint)

	hard, ok := suppression.ForEvent(suppression.EventBounced, suppression.BounceKindPermanent)
	assert.True(t, ok)
	assert.Equal(t, entsuppression.ReasonBounce, hard)

	_, soft := suppression.ForEvent(suppression.EventBounced, suppression.BounceKindTransient)
	assert.False(t, soft)
	_, other := suppression.ForEvent("email.opened", "")
	assert.False(t, other)
}

func TestEraseContactKeepsTheSuppressionRefusingTheAddress(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.InitechID)

	require.NoError(t, suppression.EraseContact(ctx, s, int64(fixtures.ContactErasableID)))

	row, err := s.Suppression().Get(ctx, fixtures.SuppressionErasableID)
	require.NoError(t, err)
	assert.Nil(t, row.ContactID)
	verdict, err := eligibility.Check(ctx, s, eligibility.ChannelEmail, row.Destination, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.False(t, verdict.Eligible)
}
