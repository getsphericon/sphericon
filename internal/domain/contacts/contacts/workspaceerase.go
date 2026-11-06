package contacts

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseContactsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Contact().Delete().Where(contact.IDIn(ids...)).Exec(ctx)
	return err
}

func EraseCustomFieldsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.CustomField().Delete().Where(customfield.IDIn(ids...)).Exec(ctx)
	return err
}
