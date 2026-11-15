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
// columns at a revision, its miss told as errGone or errStale.
type edited struct {
	id   int64
	read func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string)
	edit func(ctx context.Context, s *ent.Scoped, id, revision int64, value string) error
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
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value string) error {
			_, err := s.BrandKit().EditAt(id, revision).SetFooterText(value).Save(ctx)
			var miss *ent.BrandKitMiss
			if errors.As(err, &miss) {
				return miss.Refuse(errGone, errStale)
			}
			return err
		},
	},
	"Broadcast": {
		id: fixtures.BroadcastDraftID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.Broadcast().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), row.Name
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value string) error {
			_, err := s.Broadcast().EditAt(id, revision).SetName(value).Save(ctx)
			var miss *ent.BroadcastMiss
			if errors.As(err, &miss) {
				return miss.Refuse(errGone, errStale)
			}
			return err
		},
	},
	"Contact": {
		id: fixtures.ContactAliceID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.Contact().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), lo.FromPtr(row.FirstName)
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value string) error {
			_, err := s.Contact().EditAt(id, revision).SetFirstName(value).Save(ctx)
			var miss *ent.ContactMiss
			if errors.As(err, &miss) {
				return miss.Refuse(errGone, errStale)
			}
			return err
		},
	},
	"CustomField": {
		id: fixtures.CustomFieldAcmePlanID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.CustomField().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), row.Name
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value string) error {
			_, err := s.CustomField().EditAt(id, revision).SetName(value).Save(ctx)
			var miss *ent.CustomFieldMiss
			if errors.As(err, &miss) {
				return miss.Refuse(errGone, errStale)
			}
			return err
		},
	},
	"EmailTemplate": {
		id: fixtures.TemplateWelcomeID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.EmailTemplate().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), row.Name
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value string) error {
			_, err := s.EmailTemplate().EditAt(id, revision).SetName(value).Save(ctx)
			var miss *ent.EmailTemplateMiss
			if errors.As(err, &miss) {
				return miss.Refuse(errGone, errStale)
			}
			return err
		},
	},
	"Integration": {
		id: fixtures.IntegrationAcmeDefaultID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.Integration().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), row.Name
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value string) error {
			_, err := s.Integration().EditAt(id, revision).SetName(value).Save(ctx)
			var miss *ent.IntegrationMiss
			if errors.As(err, &miss) {
				return miss.Refuse(errGone, errStale)
			}
			return err
		},
	},
	"Segment": {
		id: fixtures.SegmentActiveID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.Segment().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), row.Name
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value string) error {
			_, err := s.Segment().EditAt(id, revision).SetName(value).Save(ctx)
			var miss *ent.SegmentMiss
			if errors.As(err, &miss) {
				return miss.Refuse(errGone, errStale)
			}
			return err
		},
	},
	"WebhookEndpoint": {
		id: fixtures.WebhookCodebasicsID,
		read: func(ctx context.Context, s *ent.Scoped, id int64) (stamp, string) {
			row := lo.Must(s.WebhookEndpoint().Get(ctx, id))
			return at(row.Revision, row.UpdatedAt), row.URL
		},
		edit: func(ctx context.Context, s *ent.Scoped, id, revision int64, value string) error {
			_, err := s.WebhookEndpoint().EditAt(id, revision).SetURL(value).Save(ctx)
			var miss *ent.WebhookEndpointMiss
			if errors.As(err, &miss) {
				return miss.Refuse(errGone, errStale)
			}
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

			err := tc.edit(ctx, s, tc.id, before.revision-1, value+" edited")

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

			assert.ErrorIs(t, tc.edit(ctx, env.DB.Scoped(acme), 999999, 1, "anything"), errGone)
			assert.ErrorIs(t, tc.edit(ctx, env.DB.Scoped(globex), tc.id, 1, "anything"), errGone, "a row of another Workspace")
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

			require.NoError(t, tc.edit(ctx, s, tc.id, before.revision, value))
			unchanged, _ := tc.read(ctx, s, tc.id)
			require.NoError(t, tc.edit(ctx, s, tc.id, before.revision, value+" edited"))
			edited, _ := tc.read(ctx, s, tc.id)

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

			assert.ErrorIs(t, tc.edit(ctx, s, tc.id, before.revision+1, value), errStale)
		})
	}
}
