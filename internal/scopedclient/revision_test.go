package scopedclient_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The edit at a revision (ADR 0119) as the templates generate it for every type with a revision but
// the Versioned definitions: a stale revision is refused and leaves the row as it was, a missing row
// is gone, and a write that changes nothing keeps the revision and updated_at, unless it is made over
// a stale revision.

var (
	errGone  = errors.New("gone")
	errStale = errors.New("stale")
)

// edited is one row of a type with a revision: what it is now, and an edit of one of its text
// columns at a revision (nil sets nothing), its miss refused as errGone or errStale.
type edited struct {
	id   int64
	read func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string)
	edit func(ctx context.Context, s *ent.Scoped, id, revision int64, value *string) error
}

// stamp is what a write of a row moves.
type stamp struct {
	revision  int64
	updatedAt time.Time
}

func at(revision int64, updatedAt time.Time) stamp { return stamp{revision, updatedAt.UTC()} }

var revised = map[string]edited{
	"BrandKit": {
		id: fixtures.BrandKitAcmeID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.BrandKit().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), row.FooterText
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value *string) error {
			_, err := s.BrandKit().EditAt(id, revision).Refusing(errGone, errStale).SetNillableFooterText(value).Save(ctx)
			return err
		},
	},
	"Broadcast": {
		id: fixtures.BroadcastDraftID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.Broadcast().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), row.Name
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value *string) error {
			_, err := s.Broadcast().EditAt(id, revision).Refusing(errGone, errStale).SetNillableName(value).Save(ctx)
			return err
		},
	},
	"Contact": {
		id: fixtures.ContactAliceID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.Contact().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), lo.FromPtr(row.FirstName)
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value *string) error {
			_, err := s.Contact().EditAt(id, revision).Refusing(errGone, errStale).SetNillableFirstName(value).Save(ctx)
			return err
		},
	},
	"CustomField": {
		id: fixtures.CustomFieldAcmePlanID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.CustomField().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), row.Name
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value *string) error {
			_, err := s.CustomField().EditAt(id, revision).Refusing(errGone, errStale).SetNillableName(value).Save(ctx)
			return err
		},
	},
	"EmailTemplate": {
		id: fixtures.TemplateWelcomeID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.EmailTemplate().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), row.Name
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value *string) error {
			_, err := s.EmailTemplate().EditAt(id, revision).Refusing(errGone, errStale).SetNillableName(value).Save(ctx)
			return err
		},
	},
	"Integration": {
		id: fixtures.IntegrationAcmeDefaultID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.Integration().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), row.Name
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value *string) error {
			_, err := s.Integration().EditAt(id, revision).Refusing(errGone, errStale).SetNillableName(value).Save(ctx)
			return err
		},
	},
	"Segment": {
		id: fixtures.SegmentActiveID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.Segment().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), row.Name
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value *string) error {
			_, err := s.Segment().EditAt(id, revision).Refusing(errGone, errStale).SetNillableName(value).Save(ctx)
			return err
		},
	},
	"WebhookEndpoint": {
		id: fixtures.WebhookCodebasicsID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.WebhookEndpoint().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), row.URL
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value *string) error {
			_, err := s.WebhookEndpoint().EditAt(id, revision).Refusing(errGone, errStale).SetNillableURL(value).Save(ctx)
			return err
		},
	},
}

// Every Workspace-owned type with a revision, a Versioned definition aside, has its row in the table.
func TestEveryRevisedTypeHasAnEditAtARevision(t *testing.T) {
	listed := lo.FilterMap(ent.ScopedEntities(), func(e *ent.ScopedEntity, _ int) (string, bool) {
		_, edits := e.Wrapper.MethodByName("EditAt")
		assert.Equalf(t, e.Revised && !e.Versioned, edits, "%s has EditAt when it has a revision", e.Name)
		return e.Name, e.Revised && !e.Versioned
	})
	assert.ElementsMatch(t, listed, lo.Keys(revised))
}

func TestAnEditAtAStaleRevisionIsRefusedAndLeavesTheRow(t *testing.T) {
	for name, tc := range revised {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			s := env.DB.Scoped(acme)
			before, value := tc.read(ctx, s, tc.id)

			err := tc.edit(ctx, s, tc.id, before.revision-1, new(value+" edited"))

			require.ErrorIs(t, err, errStale)
			after, kept := tc.read(ctx, s, tc.id)
			assert.Equal(t, before, after)
			assert.Equal(t, value, kept)
		})
	}
}

func TestAnEditOfAMissingRowIsGone(t *testing.T) {
	for name, tc := range revised {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()

			missing := tc.edit(ctx, env.DB.Scoped(acme), 999999, 1, new("anything"))
			foreign := tc.edit(ctx, env.DB.Scoped(globex), tc.id, 1, new("anything"))

			require.ErrorIs(t, missing, errGone)
			require.ErrorIs(t, foreign, errGone, "a row of another Workspace")
		})
	}
}

func TestAnEditAtARevisionMovesItAndOneThatChangesNothingDoesNot(t *testing.T) {
	for name, tc := range revised {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			s := env.DB.Scoped(acme)
			before, value := tc.read(ctx, s, tc.id)

			same := tc.edit(ctx, s, tc.id, before.revision, &value)
			unchanged, _ := tc.read(ctx, s, tc.id)
			changed := tc.edit(ctx, s, tc.id, before.revision, new(value+" edited"))
			edited, _ := tc.read(ctx, s, tc.id)

			require.NoError(t, same)
			require.NoError(t, changed)
			assert.Equal(t, before, unchanged, "a write that changes nothing keeps the revision and updated_at")
			assert.Equal(t, before.revision+1, edited.revision)
		})
	}
}

func TestAnEditThatChangesNothingAtAStaleRevisionIsRefused(t *testing.T) {
	for name, tc := range revised {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			s := env.DB.Scoped(acme)
			before, value := tc.read(ctx, s, tc.id)

			err := tc.edit(ctx, s, tc.id, before.revision+1, &value)

			require.ErrorIs(t, err, errStale)
		})
	}
}

// The scope's own Workspace is edited at a revision the same way (Scoped.EditWorkspaceAt): a stale
// revision is refused and leaves the row, a write that changes nothing keeps the revision and
// updated_at, and is refused over a stale revision.
func TestTheTenantEditAtARevision(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(acme)
	read := func() (stamp, string) {
		ws := lo.Must(s.Workspace(ctx))
		return at(ws.Revision, ws.UpdatedAt), ws.Name
	}
	edit := func(revision int64, name string) error { return renameWorkspace(ctx, s, revision, name) }
	before, name := read()

	staleEdit := edit(before.revision-1, name+" edited")
	staleNoOp := edit(before.revision+1, name)
	stale, _ := read()
	same := edit(before.revision, name)
	unchanged, _ := read()
	changed := edit(before.revision, name+" edited")
	edited, _ := read()

	require.ErrorIs(t, staleEdit, errStale)
	require.ErrorIs(t, staleNoOp, errStale, "a write that changes nothing over a stale revision")
	require.NoError(t, same)
	require.NoError(t, changed)
	assert.Equal(t, before, stale)
	assert.Equal(t, before, unchanged, "a write that changes nothing keeps the revision and updated_at")
	assert.Equal(t, before.revision+1, edited.revision)
}

// renameWorkspace renames the Workspace of s at revision, its miss told as errGone or errStale.
func renameWorkspace(ctx context.Context, s *ent.Scoped, revision int64, name string) error {
	_, err := s.EditWorkspaceAt(revision).Refusing(errGone, errStale).SetName(name).Save(ctx)
	return err
}

// An edit at a revision that sets nothing writes nothing (ADR 0119): it keeps the revision and
// updated_at, and is refused like any edit when the row has moved past the revision.
func TestAnEditThatSetsNothingWritesNothing(t *testing.T) {
	for name, tc := range revised {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			s := env.DB.Scoped(acme)
			before, _ := tc.read(ctx, s, tc.id)

			current := tc.edit(ctx, s, tc.id, before.revision, nil)
			stale := tc.edit(ctx, s, tc.id, before.revision-1, nil)
			missing := tc.edit(ctx, s, 999999, 1, nil)
			after, _ := tc.read(ctx, s, tc.id)

			require.NoError(t, current)
			require.ErrorIs(t, stale, errStale)
			require.ErrorIs(t, missing, errGone)
			assert.Equal(t, before, after)
		})
	}
}

// The scope's own Workspace likewise: an edit of it that sets nothing writes nothing.
func TestATenantEditThatSetsNothingWritesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(acme)
	before := lo.Must(s.Workspace(ctx))

	_, current := s.EditWorkspaceAt(before.Revision).Refusing(errGone, errStale).Save(ctx)
	_, stale := s.EditWorkspaceAt(before.Revision-1).Refusing(errGone, errStale).Save(ctx)
	after := lo.Must(s.Workspace(ctx))

	require.NoError(t, current)
	require.ErrorIs(t, stale, errStale)
	assert.Equal(t, at(before.Revision, before.UpdatedAt), at(after.Revision, after.UpdatedAt))
}
