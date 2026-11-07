package versioneddefinition_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/content/versioneddefinition"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// Position storage lets these properties exercise the domain's transition decisions alone;
// the generated conditional writer is tested separately against the real database.
type positionMemory struct {
	position ent.VersionPosition
}

func (m *positionMemory) storage() lifecycle.Storage[ent.VersionPosition] {
	return lifecycle.Storage[ent.VersionPosition]{
		Read: func(context.Context) (ent.VersionPosition, error) { return m.position, nil },
		Write: func(_ context.Context, _, next ent.VersionPosition) (bool, error) {
			m.position = next
			return true, nil
		},
	}
}

func drawKeptPosition(t *rapid.T) ent.VersionPosition {
	live := rapid.Int64Range(1, 1000).Draw(t, "live version")
	draft := rapid.Int64Range(1001, 2000).Draw(t, "draft version")
	return rapid.SampledFrom([]ent.VersionPosition{
		{DraftID: draft},
		{LiveID: live},
		{LiveID: live, DraftID: draft},
		{Active: true, LiveID: live},
		{Active: true, LiveID: live, DraftID: draft},
	}).Draw(t, "kept position")
}

func TestPublishAlwaysPromotesExactlyTheFormerDraft(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := drawKeptPosition(t)
		before.DraftID = rapid.Int64Range(1001, 2000).Draw(t, "published draft")
		memory := positionMemory{position: before}

		err := versioneddefinition.Transition(t.Context(), memory.storage(), versioneddefinition.Change{Trigger: versioneddefinition.Publish})

		require.NoError(t, err)
		assert.Equal(t, ent.VersionPosition{Active: true, LiveID: before.DraftID}, memory.position)
	})
}

func TestPublishWithoutADraftPreservesTheLiveVersion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := ent.VersionPosition{
			Active: rapid.Bool().Draw(t, "active"),
			LiveID: rapid.Int64Range(1, 1000).Draw(t, "live version"),
		}
		memory := positionMemory{position: before}

		err := versioneddefinition.Transition(t.Context(), memory.storage(), versioneddefinition.Change{Trigger: versioneddefinition.Publish})

		require.NoError(t, err)
		assert.Equal(t, ent.VersionPosition{Active: true, LiveID: before.LiveID}, memory.position)
	})
}

func TestAttachingADraftNeverChangesWhatIsLiveOrArmsIt(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := drawKeptPosition(t)
		nextDraft := rapid.Int64Range(2001, 3000).Draw(t, "new draft")
		memory := positionMemory{position: before}

		err := versioneddefinition.Transition(t.Context(), memory.storage(), versioneddefinition.Change{Trigger: versioneddefinition.Draft, DraftID: nextDraft})

		require.NoError(t, err)
		assert.Equal(t, ent.VersionPosition{Active: before.Active, LiveID: before.LiveID, DraftID: nextDraft}, memory.position)
	})
}

func TestRevertPreservesTheLiveVersionAndItsActivity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := ent.VersionPosition{
			Active:  rapid.Bool().Draw(t, "active"),
			LiveID:  rapid.Int64Range(1, 1000).Draw(t, "live version"),
			DraftID: rapid.Int64Range(1001, 2000).Draw(t, "draft version"),
		}
		memory := positionMemory{position: before}

		err := versioneddefinition.Transition(t.Context(), memory.storage(), versioneddefinition.Change{Trigger: versioneddefinition.Revert})

		require.NoError(t, err)
		assert.Equal(t, ent.VersionPosition{Active: before.Active, LiveID: before.LiveID}, memory.position)
	})
}

func TestANeverPublishedDefinitionCannotLoseItsOnlyDraft(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := ent.VersionPosition{DraftID: rapid.Int64Range(1, 2000).Draw(t, "only draft")}
		memory := positionMemory{position: before}

		err := versioneddefinition.Transition(t.Context(), memory.storage(), versioneddefinition.Change{Trigger: versioneddefinition.Revert})

		require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
		assert.Equal(t, before, memory.position)
	})
}

func TestArchiveAndUnarchivePreserveVersionsButNeverRearmThem(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := drawKeptPosition(t)
		memory := positionMemory{position: before}

		err := versioneddefinition.Transition(t.Context(), memory.storage(), versioneddefinition.Change{Trigger: versioneddefinition.Archive})
		require.NoError(t, err)
		assert.Equal(t, ent.VersionPosition{Archived: true, LiveID: before.LiveID, DraftID: before.DraftID}, memory.position)
		err = versioneddefinition.Transition(t.Context(), memory.storage(), versioneddefinition.Change{Trigger: versioneddefinition.Unarchive})

		require.NoError(t, err)
		assert.Equal(t, ent.VersionPosition{LiveID: before.LiveID, DraftID: before.DraftID}, memory.position)
	})
}

func TestArchivedDefinitionsRefuseEveryOwnerEdit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := drawKeptPosition(t)
		before.Active = false
		before.Archived = true
		event := rapid.SampledFrom([]versioneddefinition.Trigger{
			versioneddefinition.Draft, versioneddefinition.Edit, versioneddefinition.Publish,
			versioneddefinition.Revert, versioneddefinition.Deactivate, versioneddefinition.Discard,
		}).Draw(t, "owner move")
		memory := positionMemory{position: before}

		err := versioneddefinition.Transition(t.Context(), memory.storage(), versioneddefinition.Change{Trigger: event, DraftID: 3001})

		require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
		assert.Equal(t, before, memory.position)
	})
}

func TestDiscardedDefinitionsCannotResumeOrChangeTheirVersions(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := drawKeptPosition(t)
		memory := positionMemory{position: before}
		event := rapid.SampledFrom([]versioneddefinition.Trigger{
			versioneddefinition.Draft, versioneddefinition.Edit, versioneddefinition.Publish,
			versioneddefinition.Revert, versioneddefinition.Deactivate, versioneddefinition.Archive,
			versioneddefinition.Unarchive,
		}).Draw(t, "later move")
		err := versioneddefinition.Transition(t.Context(), memory.storage(), versioneddefinition.Change{Trigger: versioneddefinition.Discard})
		require.NoError(t, err)

		err = versioneddefinition.Transition(t.Context(), memory.storage(), versioneddefinition.Change{Trigger: event, DraftID: 3001})

		require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
		assert.Equal(t, ent.VersionPosition{Discarded: true, LiveID: before.LiveID, DraftID: before.DraftID}, memory.position)
	})
}

func TestADraftRequiresAPositiveVersionID(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := drawKeptPosition(t)
		memory := positionMemory{position: before}
		invalidID := rapid.Int64Max(0).Draw(t, "invalid version id")

		err := versioneddefinition.Transition(t.Context(), memory.storage(), versioneddefinition.Change{Trigger: versioneddefinition.Draft, DraftID: invalidID})

		require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
		assert.Equal(t, before, memory.position)
	})
}
