package unsubscriberecording_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/unsubscribes"
	"github.com/getsphericon/sphericon/internal/usecase/unsubscriberecording"
)

func TestUnsubscribeValidatesTheSourceAndIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	_, err := unsubscriberecording.New(env.Bus).Unsubscribe(ctx, s, "  ", eligibility.SourceBroadcasts)
	require.ErrorIs(t, err, unsubscribes.ErrDestinationEmpty)
	_, err = unsubscriberecording.New(env.Bus).Unsubscribe(ctx, s, fixtures.ContactAliceEmail, "newsletter")
	require.ErrorIs(t, err, unsubscriberecording.ErrInvalidSource)
	_, err = unsubscriberecording.New(env.Bus).Unsubscribe(ctx, s, fixtures.ContactAliceEmail, "automation:999999")
	require.ErrorIs(t, err, unsubscriberecording.ErrUnknownAutomation)

	first, err := unsubscriberecording.New(env.Bus).Unsubscribe(ctx, s, "Alice@Example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, fixtures.ContactAliceEmail, first.Destination)
	assert.Equal(t, eligibility.SourceBroadcasts, first.SendingSource)

	again, err := unsubscriberecording.New(env.Bus).Unsubscribe(ctx, s, fixtures.ContactAliceEmail, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID, "a repeat is a no-op")
}

func TestUnsubscribeAcceptsAnAutomationOfTheWorkspaceOnly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	source := "automation:" + strconv.Itoa(fixtures.AutomationWelcomeSeriesID)

	got, err := unsubscriberecording.New(env.Bus).Unsubscribe(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactAliceEmail, source)
	require.NoError(t, err)
	assert.Equal(t, source, got.SendingSource)

	_, err = unsubscriberecording.New(env.Bus).Unsubscribe(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.ContactAliceEmail, source)
	assert.ErrorIs(t, err, unsubscriberecording.ErrUnknownAutomation, "another Workspace's Automation is unknown")
}
