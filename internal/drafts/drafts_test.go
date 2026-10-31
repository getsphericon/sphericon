package drafts_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/ent/ruleversion"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The invariants of the lifecycle every Versioned definition shares (ADR 0065), run for each kind
// through its own module on the real database: what the drafts package decides, seen as the
// surfaces see it.

// version is one version as a test reads it: its number and the mark an edit wrote into it (an
// Automation's Trigger, a Rule's first Trigger event). The zero value is no version.
type version struct {
	number int
	mark   string
}

// view is an object's lifecycle as a test reads it.
type view struct {
	active      bool
	archived    bool
	activatedAt *time.Time
	live, draft version
}

// kind drives one Versioned definition through its module, on committed fixture objects.
type kind struct {
	// The audit target type of the object.
	target string
	// An active object with a live version and no draft, nothing in flight on it and nothing that
	// needs it; whose live mark is liveMark.
	live     int64
	liveMark string
	// An active object with a live version (number 1) and a draft (number 2).
	liveWithDraft int64
	// An object never published: its draft is its whole definition.
	neverPublished int64
	// An object of another Workspace.
	foreign int64

	edit       func(ctx context.Context, s *ent.Scoped, id int64, mark string) (view, error)
	get        func(ctx context.Context, s *ent.Scoped, id int64) (view, error)
	publish    func(ctx context.Context, s *ent.Scoped, id int64) (view, error)
	revert     func(ctx context.Context, s *ent.Scoped, id int64) (view, error)
	deactivate func(ctx context.Context, s *ent.Scoped, id int64) (view, error)
	archive    func(ctx context.Context, s *ent.Scoped, id int64) (view, error)
	unarchive  func(ctx context.Context, s *ent.Scoped, id int64) (view, error)
	// versions are the numbers of the object's versions, in order.
	versions func(t *testing.T, id int64) []int

	errNotFound, errNothingToRevert error
}

func automationVersion(v *ent.AutomationVersion) version {
	if v == nil {
		return version{}
	}
	return version{number: v.Version, mark: v.TriggerEvent}
}

func automationView(a *ent.Automation, err error) (view, error) {
	if err != nil {
		return view{}, err
	}
	live, draft := automations.LiveAndDraft(a)
	return view{
		active: a.Status == automation.StatusActive, archived: a.DiscardedAt != nil, activatedAt: a.ActivatedAt,
		live: automationVersion(live), draft: automationVersion(draft),
	}, nil
}

func automationKind(env *testhelper.TestEnv) kind {
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	return kind{
		target: "automation",
		live:   fixtures.AutomationHoldDemoID, liveMark: "hold_demo",
		liveWithDraft: fixtures.AutomationWelcomeSeriesID, neverPublished: fixtures.AutomationWinBackID,
		foreign: fixtures.AutomationGlobexID,
		edit: func(ctx context.Context, s *ent.Scoped, id int64, mark string) (view, error) {
			return automationView(m.Update(ctx, s, id, automations.UpdateInput{TriggerEvent: &mark}))
		},
		get: func(ctx context.Context, s *ent.Scoped, id int64) (view, error) {
			return automationView(m.Get(ctx, s, id))
		},
		publish: func(ctx context.Context, s *ent.Scoped, id int64) (view, error) {
			return automationView(m.Publish(ctx, s, id))
		},
		revert: func(ctx context.Context, s *ent.Scoped, id int64) (view, error) {
			return automationView(m.Revert(ctx, s, id))
		},
		deactivate: func(ctx context.Context, s *ent.Scoped, id int64) (view, error) {
			return automationView(m.Deactivate(ctx, s, id))
		},
		archive: func(ctx context.Context, s *ent.Scoped, id int64) (view, error) {
			return automationView(m.Archive(ctx, s, id))
		},
		unarchive: func(ctx context.Context, s *ent.Scoped, id int64) (view, error) {
			return automationView(m.Unarchive(ctx, s, id))
		},
		versions: func(t *testing.T, id int64) []int {
			t.Helper()
			rows, err := env.DB.AutomationVersion.Query().
				Where(automationversion.AutomationID(id)).
				Order(automationversion.ByVersion()).
				All(context.Background())
			require.NoError(t, err)
			return lo.Map(rows, func(v *ent.AutomationVersion, _ int) int { return v.Version })
		},
		errNotFound: automations.ErrNotFound, errNothingToRevert: automations.ErrNothingToRevert,
	}
}

func ruleVersion(v *ent.RuleVersion) version {
	if v == nil {
		return version{}
	}
	return version{number: v.Version, mark: rules.Triggers(v)[0]}
}

func ruleView(r *ent.Rule, err error) (view, error) {
	if err != nil {
		return view{}, err
	}
	live, draft := rules.LiveAndDraft(r)
	return view{
		active: r.Status == rule.StatusActive, archived: r.DiscardedAt != nil, activatedAt: r.ActivatedAt,
		live: ruleVersion(live), draft: ruleVersion(draft),
	}, nil
}

func ruleKind(env *testhelper.TestEnv) kind {
	m := rules.New(env.Cipher, env.Bus)
	return kind{
		target: "rule",
		live:   fixtures.RuleVipOnlyID, liveMark: "rule_vip",
		liveWithDraft: fixtures.RuleTagClickedID, neverPublished: fixtures.RuleInactiveID,
		foreign: fixtures.RuleGlobexID,
		edit: func(ctx context.Context, s *ent.Scoped, id int64, mark string) (view, error) {
			return ruleView(m.Update(ctx, s, id, rules.UpdateInput{TriggerEvents: &[]string{mark}}))
		},
		get: func(ctx context.Context, s *ent.Scoped, id int64) (view, error) {
			return ruleView(m.Get(ctx, s, id))
		},
		publish: func(ctx context.Context, s *ent.Scoped, id int64) (view, error) {
			return ruleView(m.Publish(ctx, s, id))
		},
		revert: func(ctx context.Context, s *ent.Scoped, id int64) (view, error) {
			return ruleView(m.Revert(ctx, s, id))
		},
		deactivate: func(ctx context.Context, s *ent.Scoped, id int64) (view, error) {
			return ruleView(m.Deactivate(ctx, s, id))
		},
		archive: func(ctx context.Context, s *ent.Scoped, id int64) (view, error) {
			return ruleView(m.Archive(ctx, s, id))
		},
		unarchive: func(ctx context.Context, s *ent.Scoped, id int64) (view, error) {
			return ruleView(m.Unarchive(ctx, s, id))
		},
		versions: func(t *testing.T, id int64) []int {
			t.Helper()
			rows, err := env.DB.RuleVersion.Query().
				Where(ruleversion.RuleID(id)).
				Order(ruleversion.ByVersion()).
				All(context.Background())
			require.NoError(t, err)
			return lo.Map(rows, func(v *ent.RuleVersion, _ int) int { return v.Version })
		},
		errNotFound: rules.ErrNotFound, errNothingToRevert: rules.ErrNothingToRevert,
	}
}

// kinds are the Versioned definitions, each built on a test's environment: every test runs for
// each, in a transaction of its own.
var kinds = []struct {
	name  string
	build func(env *testhelper.TestEnv) kind
}{
	{"automation", automationKind},
	{"rule", ruleKind},
}

func acme(env *testhelper.TestEnv) *ent.Scoped { return env.DB.Scoped(fixtures.AcmeID) }

// #748: an edit of a live object writes a draft above its live version and leaves the live one as
// it is, so it changes nothing that acts until it is published.
func TestAnEditOfALiveObjectMakesADraftAndLeavesLive(t *testing.T) {
	for _, row := range kinds {
		t.Run(row.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			k := row.build(env)
			ctx := context.Background()

			got, err := k.edit(ctx, acme(env), k.live, "drafts-edited")

			require.NoError(t, err)
			assert.Equal(t, version{number: 1, mark: k.liveMark}, got.live, "the live version is never rewritten")
			assert.Equal(t, version{number: 2, mark: "drafts-edited"}, got.draft, "the draft is the version above the newest")
			assert.True(t, got.active, "an edit leaves the status alone")
		})
	}
}

func TestASecondEditRewritesTheDraftInPlace(t *testing.T) {
	for _, row := range kinds {
		t.Run(row.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			k := row.build(env)
			ctx := context.Background()
			_, err := k.edit(ctx, acme(env), k.live, "drafts-first")
			require.NoError(t, err)

			got, err := k.edit(ctx, acme(env), k.live, "drafts-second")

			require.NoError(t, err)
			assert.Equal(t, version{number: 2, mark: "drafts-second"}, got.draft)
			assert.Equal(t, []int{1, 2}, k.versions(t, k.live), "one draft, rewritten in place")
		})
	}
}

// A never-published object's draft is its whole definition: every edit rewrites it, and nothing is
// live.
func TestEditingANeverPublishedObjectRewritesItsDraft(t *testing.T) {
	for _, row := range kinds {
		t.Run(row.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			k := row.build(env)
			ctx := context.Background()
			_, err := k.edit(ctx, acme(env), k.neverPublished, "drafts-first")
			require.NoError(t, err)

			got, err := k.edit(ctx, acme(env), k.neverPublished, "drafts-second")

			require.NoError(t, err)
			assert.Equal(t, version{}, got.live, "nothing is live until it is published")
			assert.Equal(t, version{number: 1, mark: "drafts-second"}, got.draft)
			assert.Equal(t, []int{1}, k.versions(t, k.neverPublished))
		})
	}
}

// The editor sends every field: saving the live definition unchanged stores nothing.
func TestAnUnchangedEditStoresNothing(t *testing.T) {
	for _, row := range kinds {
		t.Run(row.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			k := row.build(env)
			got, err := k.edit(context.Background(), acme(env), k.live, k.liveMark)

			require.NoError(t, err)
			assert.Equal(t, version{}, got.draft)
			assert.Equal(t, []int{1}, k.versions(t, k.live))
		})
	}
}

// Publish makes the draft live and keeps the old live version as history; an object already
// active keeps its activation time.
func TestPublishMakesTheDraftLiveAndKeepsTheOldLiveVersion(t *testing.T) {
	for _, row := range kinds {
		t.Run(row.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			k := row.build(env)
			ctx := context.Background()
			edited, err := k.edit(ctx, acme(env), k.live, "drafts-published")
			require.NoError(t, err)

			got, err := k.publish(ctx, acme(env), k.live)

			require.NoError(t, err)
			assert.Equal(t, version{number: 2, mark: "drafts-published"}, got.live)
			assert.Equal(t, version{}, got.draft, "the draft became live")
			assert.True(t, got.active)
			assert.Equal(t, edited.activatedAt, got.activatedAt, "republishing an active object keeps its activation time")
			assert.Equal(t, []int{1, 2}, k.versions(t, k.live), "the old live version stays as history")
		})
	}
}

// With no draft, Publish makes a deactivated object active again on the same live version, from now.
func TestPublishWithoutADraftReactivatesTheLiveVersion(t *testing.T) {
	for _, row := range kinds {
		t.Run(row.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			k := row.build(env)
			ctx := context.Background()
			start := time.Now()
			stopped, err := k.deactivate(ctx, acme(env), k.live)
			require.NoError(t, err)
			require.False(t, stopped.active)

			got, err := k.publish(ctx, acme(env), k.live)

			require.NoError(t, err)
			assert.True(t, got.active)
			assert.Equal(t, version{number: 1, mark: k.liveMark}, got.live)
			assert.Equal(t, []int{1}, k.versions(t, k.live))
			require.NotNil(t, got.activatedAt)
			assert.False(t, got.activatedAt.Before(start), "the object acts from the moment it is published again")
		})
	}
}

// Revert drops the draft above a live version, and only that: an object with no draft, or one never
// published (whose draft is its whole definition), has nothing to revert.
func TestRevertDropsTheDraftAboveALiveVersionOnly(t *testing.T) {
	for _, row := range kinds {
		t.Run(row.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			k := row.build(env)
			ctx := context.Background()
			_, noDraft := k.revert(ctx, acme(env), k.live)
			_, neverPublished := k.revert(ctx, acme(env), k.neverPublished)

			got, err := k.revert(ctx, acme(env), k.liveWithDraft)
			require.NoError(t, err)
			_, again := k.revert(ctx, acme(env), k.liveWithDraft)

			require.ErrorIs(t, noDraft, k.errNothingToRevert)
			require.ErrorIs(t, neverPublished, k.errNothingToRevert)
			assert.Equal(t, version{}, got.draft)
			assert.Equal(t, 1, got.live.number)
			assert.Equal(t, []int{1}, k.versions(t, k.liveWithDraft), "the draft's version goes")
			require.ErrorIs(t, again, k.errNothingToRevert, "there is no draft any more")
			after, err := k.get(ctx, acme(env), k.neverPublished)
			require.NoError(t, err)
			assert.Equal(t, 1, after.draft.number, "a never-published object keeps its draft")
		})
	}
}

// The edit after a Revert is the version above the live one again.
func TestAnEditAfterRevertMakesANewDraft(t *testing.T) {
	for _, row := range kinds {
		t.Run(row.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			k := row.build(env)
			ctx := context.Background()
			_, err := k.revert(ctx, acme(env), k.liveWithDraft)
			require.NoError(t, err)

			got, err := k.edit(ctx, acme(env), k.liveWithDraft, "drafts-again")

			require.NoError(t, err)
			assert.Equal(t, version{number: 2, mark: "drafts-again"}, got.draft)
		})
	}
}

// Every move of another Workspace's object is not found.
func TestAnotherWorkspacesObjectIsNotFound(t *testing.T) {
	for _, row := range kinds {
		t.Run(row.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			k := row.build(env)
			ctx := context.Background()
			s := acme(env)

			_, getErr := k.get(ctx, s, k.foreign)
			_, editErr := k.edit(ctx, s, k.foreign, "drafts-foreign")
			_, publishErr := k.publish(ctx, s, k.foreign)
			_, revertErr := k.revert(ctx, s, k.foreign)
			_, deactivateErr := k.deactivate(ctx, s, k.foreign)
			_, archiveErr := k.archive(ctx, s, k.foreign)
			_, unarchiveErr := k.unarchive(ctx, s, k.foreign)

			require.ErrorIs(t, getErr, k.errNotFound)
			require.ErrorIs(t, editErr, k.errNotFound)
			require.ErrorIs(t, publishErr, k.errNotFound)
			require.ErrorIs(t, revertErr, k.errNotFound)
			require.ErrorIs(t, deactivateErr, k.errNotFound)
			require.ErrorIs(t, archiveErr, k.errNotFound)
			require.ErrorIs(t, unarchiveErr, k.errNotFound)
		})
	}
}

// Making a draft, publishing it and reverting the next one move the object's pointers, so each is an
// audited write of the object (the versions are not audited themselves).
func TestTheMovesAreAudited(t *testing.T) {
	for _, row := range kinds {
		t.Run(row.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			k := row.build(env)
			ctx := context.Background()
			actor := events.Actor{Kind: events.ActorUser, ID: strconv.Itoa(fixtures.OwnerJohnID), Name: "John"}
			s := env.Bus.Act(acme(env), actor)
			_, err := k.edit(ctx, s, k.live, "drafts-audited")
			require.NoError(t, err)
			_, err = k.publish(ctx, s, k.live)
			require.NoError(t, err)
			_, err = k.edit(ctx, s, k.live, "drafts-audited-again")
			require.NoError(t, err)
			_, err = k.revert(ctx, s, k.live)
			require.NoError(t, err)

			entries := lo.FilterMap(env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) (*events.AuditEntry, bool) {
				e, ok := ev.(*events.AuditEntry)
				return e, ok && e.TargetType == k.target && e.TargetID == strconv.FormatInt(k.live, 10)
			})
			diffs := lo.Map(entries, func(e *events.AuditEntry, _ int) map[string]any { return e.Diff })

			require.Len(t, diffs, 4, "the draft made, published, made again and reverted")
			assert.Contains(t, diffs[0], automation.FieldDraftVersionID)
			assert.Contains(t, diffs[1], automation.FieldLiveVersionID)
			assert.Contains(t, diffs[1], automation.FieldDraftVersionID)
			assert.Contains(t, diffs[2], automation.FieldDraftVersionID)
			assert.Contains(t, diffs[3], automation.FieldDraftVersionID)
		})
	}
}

// Archive stops the object before it leaves the lists, so Unarchive brings it back inactive: only
// Publish arms it again (an Automation archived active used to come back enrolling).
func TestArchiveLeavesTheObjectInactiveAndUnarchiveDoesNotRearmIt(t *testing.T) {
	for _, row := range kinds {
		t.Run(row.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			k := row.build(env)
			ctx := context.Background()
			before, err := k.get(ctx, acme(env), k.live)
			require.NoError(t, err)
			require.True(t, before.active)

			archived, err := k.archive(ctx, acme(env), k.live)
			require.NoError(t, err)
			_, publishErr := k.publish(ctx, acme(env), k.live)
			again, err := k.archive(ctx, acme(env), k.live)
			require.NoError(t, err, "archiving an archived object changes nothing")
			restored, err := k.unarchive(ctx, acme(env), k.live)
			require.NoError(t, err)
			stored, err := k.get(ctx, acme(env), k.live)
			require.NoError(t, err)

			assert.Equal(t, archived, again, "archiving it again returns it as it is")
			assert.Equal(t, stored, restored, "Unarchive returns the object as it is stored")

			assert.True(t, archived.archived)
			assert.False(t, archived.active, "an archived object acts on nothing")
			require.ErrorIs(t, publishErr, k.errNotFound, "an archived object is restored before it is published")
			assert.False(t, restored.archived)
			assert.False(t, restored.active, "restoring does not re-arm it")
			assert.Equal(t, before.live, restored.live, "its live version stays")
		})
	}
}
