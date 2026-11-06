package contacts

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
)

// EraseContactsIn removes the scoped Workspace's Contacts with the ids, inside the transaction s is
// bound to: one batch of the erase of a deleted Workspace (internal/usecase/workspacedeletion, ADR
// 0043), which lists the ids and checks nothing else.
func EraseContactsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Contact().Delete().Where(contact.IDIn(ids...)).Exec(ctx)
	return err
}

// EraseCustomFieldsIn removes the scoped Workspace's Custom fields with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseCustomFieldsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.CustomField().Delete().Where(customfield.IDIn(ids...)).Exec(ctx)
	return err
}
