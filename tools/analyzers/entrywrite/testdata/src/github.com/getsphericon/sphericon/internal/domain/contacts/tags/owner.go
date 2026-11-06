package tags

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// The owner of Tag writes it, and its own Delete builder may be a type argument.
func remove(ctx context.Context, s *ent.Scoped) error {
	_, err := del(s.Tag().Delete()).Exec(ctx)
	return err
}

func del[D interface{ Where(...int64) D }](d D) D { return d.Where(1) }

// Tagging a Contact writes the Contact's row and its join rows: contacts owns them.
func apply(ctx context.Context, s *ent.Scoped) error {
	return s.Contact().UpdateOneID(1).AddTagIDs(2).Exec(ctx) // want `^rule 5: internal/domain/contacts/tags writes Contact \(UpdateOneID on Contact\), a table internal/domain/contacts/contacts owns; fix: give internal/domain/contacts/contacts a function that makes the write`
}
