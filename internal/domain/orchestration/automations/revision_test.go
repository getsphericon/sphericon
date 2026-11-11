package automations_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Two Saves made on one revision: the second is refused with ErrRevisionChanged, and the draft's
// steps are the first's (ADR 0119).
func TestASaveOnARevisionAlreadyMovedPastIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	read := env.AutomationRevision(ctx, t, fixtures.AutomationWinBackID)
	first := []automationstep.Step{{Type: automationstep.ApplyTag, Tag: "first"}}
	second := []automationstep.Step{{Type: automationstep.ApplyTag, Tag: "second"}}

	saved, err := m.Update(ctx, s, fixtures.AutomationWinBackID, automations.UpdateInput{Revision: read, Steps: &first})
	require.NoError(t, err)
	_, err = m.Update(ctx, s, fixtures.AutomationWinBackID, automations.UpdateInput{Revision: read, Steps: &second})

	require.ErrorIs(t, err, automations.ErrRevisionChanged)
	stored, err := m.Get(ctx, s, fixtures.AutomationWinBackID)
	require.NoError(t, err)
	assert.Equal(t, first, draftSteps(t, stored))
	assert.Equal(t, saved.Revision, stored.Revision)
}

// A Save that renames the Automation and edits its draft is one write of the row: it moves the
// revision once.
func TestARenameAndADraftEditMoveTheRevisionOnce(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	read := env.AutomationRevision(ctx, t, fixtures.AutomationWinBackID)
	name := "Renamed"
	steps := []automationstep.Step{{Type: automationstep.ApplyTag, Tag: "renamed"}}

	saved, err := m.Update(ctx, s, fixtures.AutomationWinBackID, automations.UpdateInput{Revision: read, Name: &name, Steps: &steps})

	require.NoError(t, err)
	assert.Equal(t, "Renamed", saved.Name)
	assert.Equal(t, steps, draftSteps(t, saved))
	assert.Equal(t, read+1, saved.Revision)
}

// Publish made on a stale revision is refused and publishes nothing.
func TestAPublishOnARevisionAlreadyMovedPastPublishesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	read := env.AutomationRevision(ctx, t, fixtures.AutomationWinBackID)
	name := "Moved meanwhile"
	_, err := m.Update(ctx, s, fixtures.AutomationWinBackID, automations.UpdateInput{Revision: read, Name: &name})
	require.NoError(t, err)

	_, err = m.Publish(ctx, s, fixtures.AutomationWinBackID, read)

	require.ErrorIs(t, err, automations.ErrRevisionChanged)
	stored, err := m.Get(ctx, s, fixtures.AutomationWinBackID)
	require.NoError(t, err)
	live, _ := automations.LiveAndDraft(stored)
	assert.Nil(t, live, "nothing went live")
}
