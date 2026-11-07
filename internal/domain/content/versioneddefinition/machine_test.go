package versioneddefinition_test

import (
	"context"
	"errors"
	"testing"

	"github.com/qmuntal/stateless"
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

func (m *positionMemory) machine(change versioneddefinition.Change) *stateless.StateMachine {
	return versioneddefinition.Machine(m.position, change,
		func(_ context.Context, before, next ent.VersionPosition) (bool, error) {
			if m.position != before {
				return false, nil
			}
			m.position = next
			return true, nil
		})
}

func (m *positionMemory) fire(ctx context.Context, change versioneddefinition.Change) error {
	machine := m.machine(change)
	return machine.FireCtx(ctx, change.Trigger)
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

		err := memory.fire(t.Context(), versioneddefinition.Change{Trigger: versioneddefinition.Publish})

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

		err := memory.fire(t.Context(), versioneddefinition.Change{Trigger: versioneddefinition.Publish})

		require.NoError(t, err)
		assert.Equal(t, ent.VersionPosition{Active: true, LiveID: before.LiveID}, memory.position)
	})
}

func TestAttachingADraftNeverChangesWhatIsLiveOrArmsIt(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := drawKeptPosition(t)
		nextDraft := rapid.Int64Range(2001, 3000).Draw(t, "new draft")
		memory := positionMemory{position: before}

		err := memory.fire(t.Context(), versioneddefinition.Change{Trigger: versioneddefinition.Draft, DraftID: nextDraft})

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

		err := memory.fire(t.Context(), versioneddefinition.Change{Trigger: versioneddefinition.Revert})

		require.NoError(t, err)
		assert.Equal(t, ent.VersionPosition{Active: before.Active, LiveID: before.LiveID}, memory.position)
	})
}

func TestANeverPublishedDefinitionCannotLoseItsOnlyDraft(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := ent.VersionPosition{DraftID: rapid.Int64Range(1, 2000).Draw(t, "only draft")}
		memory := positionMemory{position: before}

		err := memory.fire(t.Context(), versioneddefinition.Change{Trigger: versioneddefinition.Revert})

		require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
		assert.Equal(t, before, memory.position)
	})
}

func TestArchiveAndUnarchivePreserveVersionsButNeverRearmThem(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := drawKeptPosition(t)
		memory := positionMemory{position: before}

		err := memory.fire(t.Context(), versioneddefinition.Change{Trigger: versioneddefinition.Archive})
		require.NoError(t, err)
		assert.Equal(t, ent.VersionPosition{Archived: true, LiveID: before.LiveID, DraftID: before.DraftID}, memory.position)
		err = memory.fire(t.Context(), versioneddefinition.Change{Trigger: versioneddefinition.Unarchive})

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

		err := memory.fire(t.Context(), versioneddefinition.Change{Trigger: event, DraftID: 3001})

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
		err := memory.fire(t.Context(), versioneddefinition.Change{Trigger: versioneddefinition.Discard})
		require.NoError(t, err)

		err = memory.fire(t.Context(), versioneddefinition.Change{Trigger: event, DraftID: 3001})

		require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
		assert.Equal(t, ent.VersionPosition{Discarded: true, LiveID: before.LiveID, DraftID: before.DraftID}, memory.position)
	})
}

func TestADraftRequiresAPositiveVersionID(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := drawKeptPosition(t)
		memory := positionMemory{position: before}
		invalidID := rapid.Int64Max(0).Draw(t, "invalid version id")

		err := memory.fire(t.Context(), versioneddefinition.Change{Trigger: versioneddefinition.Draft, DraftID: invalidID})

		require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
		assert.Equal(t, before, memory.position)
	})
}

// These are internal actions, not reentries: activity hooks must not run when only the
// definition changes, even though persisting that change is still mandatory.
func TestDefinitionChangesDoNotExitOrEnterActivity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := ent.VersionPosition{
			Active:  rapid.Bool().Draw(t, "active"),
			LiveID:  rapid.Int64Range(1, 1000).Draw(t, "live version"),
			DraftID: rapid.Int64Range(1001, 2000).Draw(t, "draft version"),
		}
		event := rapid.SampledFrom([]versioneddefinition.Trigger{
			versioneddefinition.Edit, versioneddefinition.Draft,
			versioneddefinition.Revert, versioneddefinition.Publish,
		}).Draw(t, "definition change")
		before.Active = before.Active || event == versioneddefinition.Publish
		memory := positionMemory{position: before}
		machine := memory.machine(versioneddefinition.Change{Trigger: event, DraftID: 3001})
		var hooks []string
		for _, active := range []bool{false, true} {
			machine.Configure(active).
				OnEntry(func(context.Context, ...any) error { hooks = append(hooks, "entry"); return nil }).
				OnExit(func(context.Context, ...any) error { hooks = append(hooks, "exit"); return nil })
		}

		err := machine.FireCtx(t.Context(), event)

		require.NoError(t, err)
		assert.Empty(t, hooks)
		assert.Equal(t, before.Active, memory.position.Active)
	})
}

func TestPublishingAnInactiveDefinitionEntersActiveAfterPersistence(t *testing.T) {
	before := ent.VersionPosition{DraftID: 1}
	memory := positionMemory{position: before}
	machine := memory.machine(versioneddefinition.Change{Trigger: versioneddefinition.Publish})
	var hooks []string
	machine.Configure(false).OnExit(func(context.Context, ...any) error {
		hooks = append(hooks, "exit inactive")
		return nil
	})
	machine.Configure(true).OnEntry(func(context.Context, ...any) error {
		assert.Equal(t, ent.VersionPosition{Active: true, LiveID: before.DraftID}, memory.position)
		hooks = append(hooks, "enter active")
		return nil
	})

	err := machine.FireCtx(t.Context(), versioneddefinition.Publish)

	require.NoError(t, err)
	assert.Equal(t, []string{"exit inactive", "enter active"}, hooks)
	state, err := machine.State(t.Context())
	require.NoError(t, err)
	assert.Equal(t, true, state)
}

func TestBothInternalAndActivityChangesRefuseAStaleSnapshot(t *testing.T) {
	cases := []struct {
		name   string
		active bool
		change versioneddefinition.Change
	}{
		{"activation", false, versioneddefinition.Change{Trigger: versioneddefinition.Publish}},
		{"internal publication", true, versioneddefinition.Change{Trigger: versioneddefinition.Publish}},
		{"edit", true, versioneddefinition.Change{Trigger: versioneddefinition.Edit}},
		{"attach draft", true, versioneddefinition.Change{Trigger: versioneddefinition.Draft, DraftID: 4}},
		{"revert", true, versioneddefinition.Change{Trigger: versioneddefinition.Revert}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := ent.VersionPosition{Active: tc.active, LiveID: 1, DraftID: 2}
			memory := positionMemory{position: before}
			machine := memory.machine(tc.change)
			entered := false
			machine.Configure(true).OnEntry(func(context.Context, ...any) error {
				entered = true
				return nil
			})
			// A competing edit replaced the draft after this operation loaded its snapshot.
			memory.position.DraftID = 3
			concurrent := memory.position

			err := machine.FireCtx(t.Context(), tc.change.Trigger)

			require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
			assert.Equal(t, concurrent, memory.position)
			assert.False(t, entered)
		})
	}
}

func TestInternalAndActivityChangesPreserveStorageErrors(t *testing.T) {
	fault := errors.New("storage unavailable")
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "activation", true: "internal publication"}[active], func(t *testing.T) {
			before := ent.VersionPosition{Active: active, LiveID: 1, DraftID: 2}
			machine := versioneddefinition.Machine(before, versioneddefinition.Change{Trigger: versioneddefinition.Publish},
				func(context.Context, ent.VersionPosition, ent.VersionPosition) (bool, error) { return false, fault })

			err := machine.FireCtx(t.Context(), versioneddefinition.Publish)

			require.ErrorIs(t, err, fault)
		})
	}
}
