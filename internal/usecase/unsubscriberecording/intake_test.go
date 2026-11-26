package unsubscriberecording_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/unsubscriberecording"
)

func TestUnsubscribeValidatesTheSourceAndIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)

	_, err := testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, s, "  ", eligibility.SourceBroadcasts)
	require.ErrorIs(t, err, unsubscribes.ErrDestinationEmpty)
	_, err = testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, s, fixtures.ContactAliceEmail, "newsletter")
	require.ErrorIs(t, err, unsubscriberecording.ErrInvalidSource)
	_, err = testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, s, fixtures.ContactAliceEmail, "automation:999999")
	require.ErrorIs(t, err, unsubscriberecording.ErrUnknownAutomation)

	first, err := testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, s, "Alice@Example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, fixtures.ContactAliceEmail, first.Destination)
	assert.Equal(t, eligibility.SourceBroadcasts, first.SendingSource)

	again, err := testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, s, fixtures.ContactAliceEmail, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID, "a repeat is a no-op")
}

func TestUnsubscribeAcceptsAnAutomationOfTheWorkspaceOnly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	source := "automation:" + strconv.Itoa(fixtures.AutomationWelcomeSeriesID)

	got, err := testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactAliceEmail, source)
	require.NoError(t, err)
	assert.Equal(t, source, got.SendingSource)

	_, err = testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.ContactAliceEmail, source)
	assert.ErrorIs(t, err, unsubscriberecording.ErrUnknownAutomation, "another Workspace's Automation is unknown")
}
