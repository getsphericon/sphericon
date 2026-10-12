package consent_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/consent"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSuppressNormalizesAndKeepsTheExistingReason(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	got, err := consent.Suppress(ctx, s, "  Alice@Example.COM ")
	require.NoError(t, err)
	assert.Equal(t, fixtures.ContactAliceEmail, got.Destination)
	assert.Equal(t, suppression.ReasonManual, got.Reason)

	bounced, err := consent.Suppress(ctx, s, fixtures.SuppressionGhostBounceDestination)
	require.NoError(t, err)
	assert.Equal(t, suppression.ReasonBounce, bounced.Reason, "an existing entry is returned unchanged")
	assert.Equal(t, int64(fixtures.SuppressionGhostBounceID), int64(bounced.ID))
}

func TestSuppressRefusesABlankDestination(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := consent.Suppress(context.Background(), env.DB.Scoped(fixtures.AcmeID), "   ")
	assert.ErrorIs(t, err, consent.ErrDestinationEmpty)
}

func TestSuppressIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	got, err := consent.Suppress(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.ContactAliceEmail)
	require.NoError(t, err)
	assert.Equal(t, int64(fixtures.GlobexID), got.WorkspaceID)

	acme, err := env.DB.Scoped(fixtures.AcmeID).Suppression().Query().
		Where(suppression.DestinationEQ(fixtures.ContactAliceEmail)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, acme, "another Workspace's Suppression does not leak")
}

func TestUnsubscribeValidatesTheSourceAndIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	_, err := consent.Unsubscribe(ctx, env.Bus, s, "  ", eligibility.SourceBroadcasts)
	assert.ErrorIs(t, err, consent.ErrDestinationEmpty)
	_, err = consent.Unsubscribe(ctx, env.Bus, s, fixtures.ContactAliceEmail, "newsletter")
	assert.ErrorIs(t, err, consent.ErrInvalidSource)
	_, err = consent.Unsubscribe(ctx, env.Bus, s, fixtures.ContactAliceEmail, "automation:999999")
	assert.ErrorIs(t, err, consent.ErrUnknownAutomation)

	first, err := consent.Unsubscribe(ctx, env.Bus, s, "Alice@Example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, fixtures.ContactAliceEmail, first.Destination)
	assert.Equal(t, eligibility.SourceBroadcasts, first.SendingSource)

	again, err := consent.Unsubscribe(ctx, env.Bus, s, fixtures.ContactAliceEmail, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID, "a repeat is a no-op")
}

func TestUnsubscribeAcceptsAnAutomationOfTheWorkspaceOnly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	source := "automation:" + strconv.Itoa(fixtures.AutomationWelcomeSeriesID)

	got, err := consent.Unsubscribe(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactAliceEmail, source)
	require.NoError(t, err)
	assert.Equal(t, source, got.SendingSource)

	_, err = consent.Unsubscribe(ctx, env.Bus, env.DB.Scoped(fixtures.GlobexID), fixtures.ContactAliceEmail, source)
	assert.ErrorIs(t, err, consent.ErrUnknownAutomation, "another Workspace's Automation is unknown")
}
