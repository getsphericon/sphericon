// Package tagsunchecked holds the tags module's unchecked removal (ADR 0110): deleting a Tag asks
// whether a Segment, an Automation, a Rule or a Broadcast names it, modules above tags, so the check
// is the use case internal/usecase/tagdeletion's, the one kind of package easement lets import this
// one. The tags module does not import it.
package tagsunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
)

// DeleteIn removes the Tag from the catalogue, and from every Contact that has it, inside the
// transaction l holds the Workspace's lock in (ADR 0108). It does not ask whether a Segment, an
// Automation, a Rule or a Broadcast names the Tag: internal/usecase/tagdeletion runs the Removal
// check (the Tag's Definition use) first under the same lock. A missing Tag is tags.ErrNotFound.
func DeleteIn(ctx context.Context, l *ent.WorkspaceLocked, id int64) error {
	err := l.Tag().DeleteOneID(id).Exec(ctx)
	if ent.IsNotFound(err) {
		return tags.ErrNotFound
	}
	return err
}
