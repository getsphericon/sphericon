package suppression_test

import (
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

	got, err := suppression.Add(ctx, s, suppression.Entry{Destination: "  Blocked@Example.COM ", Reason: entsuppression.ReasonManual})
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

	got, err := suppression.Add(ctx, s, suppression.Entry{Destination: fixtures.SuppressionGhostBounceDestination, Reason: entsuppression.ReasonManual})
	require.NoError(t, err)
	assert.Equal(t, entsuppression.ReasonBounce, got.Reason)
	assert.Equal(t, int64(fixtures.SuppressionGhostBounceID), int64(got.ID))
}

func TestAddRefusesABlankDestination(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := suppression.Add(t.Context(), env.DB.Scoped(fixtures.AcmeID), suppression.Entry{Destination: "   ", Reason: entsuppression.ReasonManual})
	assert.ErrorIs(t, err, suppression.ErrDestinationEmpty)
}

func TestAddIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	got, err := suppression.Add(ctx, env.DB.Scoped(fixtures.GlobexID), suppression.Entry{Destination: fixtures.ContactAliceEmail, Reason: entsuppression.ReasonManual})
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

	all, err := suppression.List(ctx, s, pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, err)
	first, err := suppression.List(ctx, s, pagination.Params{Page: 1, PageSize: 2})
	require.NoError(t, err)
	assert.Len(t, first.Items, 2)
	assert.Equal(t, all.TotalItems, first.TotalItems)
	assert.Equal(t, all.Items[0].ID, first.Items[0].ID)
	assert.Greater(t, all.Items[0].ID, all.Items[1].ID)
}

func TestRemoveDeletesAndReportsAMissingRow(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)

	added, err := suppression.Add(ctx, s, suppression.Entry{Destination: "gone@example.com", Reason: entsuppression.ReasonManual})
	require.NoError(t, err)
	require.NoError(t, suppression.Remove(ctx, s, added.ID))

	err = suppression.Remove(ctx, s, added.ID)
	assert.ErrorIs(t, err, suppression.ErrNotFound)
}

func TestNormalizeDestinationIsIdempotentAndFoldsCase(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.String().Draw(t, "raw")
		once := suppression.NormalizeDestination(raw)
		assert.Equal(t, once, suppression.NormalizeDestination(once))
		assert.Equal(t, once, suppression.NormalizeDestination("  "+raw+"\t"))
	})
	assert.Equal(t, "a@x.com", suppression.NormalizeDestination(" A@X.com "))
}
