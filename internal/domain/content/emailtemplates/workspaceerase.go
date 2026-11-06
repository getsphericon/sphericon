package emailtemplates

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/emailtemplate"
)

// EraseEmailTemplatesIn removes the scoped Workspace's Templates with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseEmailTemplatesIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.EmailTemplate().Delete().Where(emailtemplate.IDIn(ids...)).Exec(ctx)
	return err
}
