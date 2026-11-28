package contacts

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
)

// Newest is the id of the Workspace's newest Contact, 0 when it has none. A Contact made after the
// call has a greater id: a CSV import's run reads it when it starts, to tell the Contacts it makes
// from those the Workspace had (ADR 0138).
func (m *Module) Newest(ctx context.Context, s *ent.Scoped) (int64, error) {
	ids, err := s.Contact().Query().Order(ent.Desc(contact.FieldID)).Limit(1).IDs(ctx)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return ids[0], nil
}
