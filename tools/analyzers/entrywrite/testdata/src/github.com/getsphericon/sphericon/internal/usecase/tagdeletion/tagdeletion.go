package tagdeletion

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// A use case writes no table itself: every write goes through the owner.
func remove(ctx context.Context, s *ent.Scoped, client *ent.Client) error {
	if _, err := s.Tag().Delete().Exec(ctx); err != nil { // want `^rule 5: internal/usecase/tagdeletion writes Tag \(Delete on Tag\), a table internal/tags owns`
		return err
	}
	if err := s.Segment().Archive(ctx, 1); err != nil { // want `\(Archive on Segment\), a table internal/segments owns`
		return err
	}
	if _, err := s.Segment().Purge().Exec(ctx); err != nil { // want `\(Purge on Segment\), a table internal/segments owns`
		return err
	}
	if err := s.Automation().Pointers().SetDraft(ctx, 1, 2); err != nil { // want `\(Pointers on Automation\), a table internal/automations owns`
		return err
	}
	return client.Ghost().Create().Save(ctx) // want `^rule 5: a write to Ghost \(Create on Ghost\), an entity the ownership map names no owner of; fix: add Ghost to owners`
}

// A write through a generic helper is the write of the builder it is instantiated with.
func generic(ctx context.Context, s *ent.Scoped) error {
	_, err := del(s.Tag().Delete()).Exec(ctx) // want `^rule 5: internal/usecase/tagdeletion writes Tag \(del with TagScopedDelete\), a table internal/tags owns` `\(Delete on Tag\)`
	return err
}

func del[D interface{ Where(...int64) D }](d D) D { return d.Where(1) }

// A read is not a write.
func reads(ctx context.Context, s *ent.Scoped) error {
	_, err := s.Tag().Query().All(ctx)
	return err
}
