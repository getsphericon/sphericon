package workspacedeletion

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// The Workspace erase removes a scoped table's rows itself, whichever module owns the table.
func erase(ctx context.Context, s *ent.Scoped) error {
	if _, err := s.Tag().Delete().Where(1).Exec(ctx); err != nil {
		return err
	}
	if _, err := s.Segment().Purge().Exec(ctx); err != nil {
		return err
	}
	_, err := del(s.Tag().Delete()).Exec(ctx)
	return err
}

func del[D interface{ Where(...int64) D }](d D) D { return d.Where(1) }

// It removes rows and nothing else, and only through the scope of the Workspace it erases.
func writes(ctx context.Context, s *ent.Scoped, client *ent.Client) error {
	if err := s.Tag().Create().Save(ctx); err != nil { // want `^rule 5: internal/usecase/workspacedeletion writes Tag \(Create on Tag\), a table internal/domain/contacts/tags owns`
		return err
	}
	if err := s.Segment().Archive(ctx, 1); err != nil { // want `\(Archive on Segment\), a table internal/domain/contacts/segments owns`
		return err
	}
	if err := s.Contact().UpdateOneID(1).Exec(ctx); err != nil { // want `\(UpdateOneID on Contact\), a table internal/domain/contacts/contacts owns`
		return err
	}
	return client.Tag.DeleteOneID(1).Exec(ctx) // want `\(DeleteOneID on Tag\), a table internal/domain/contacts/tags owns`
}
