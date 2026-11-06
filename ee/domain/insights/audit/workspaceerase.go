package audit

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
)

// EraseAuditEntriesIn removes the scoped Workspace's Audit entries with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseAuditEntriesIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.AuditEntry().Delete().Where(auditentry.IDIn(ids...)).Exec(ctx)
	return err
}
