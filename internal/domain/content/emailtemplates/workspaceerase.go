package emailtemplates

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/emailtemplate"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseEmailTemplatesIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.EmailTemplate().Delete().Where(emailtemplate.IDIn(ids...)).Exec(ctx)
	return err
}
