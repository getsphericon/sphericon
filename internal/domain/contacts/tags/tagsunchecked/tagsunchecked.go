// Package tagsunchecked holds the tags module's unchecked removal (ADR 0110, ADR 0145): deleting a
// Tag asks whether a Segment, an Automation, a Rule or a Broadcast names it, modules above tags, so
// that check is the use case internal/usecase/tagdeletion's, the one kind of package easement lets
// import this one, and which builds its Module. Every rule of the Tag's own row stays here. The tags
// module does not import it.
package tagsunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
)

// Module is the tags module's unchecked removal: it opens a Deletion on the Tag's row and applies
// it once the use case's check has passed (docs/architecture/modules.md, the unchecked operations).
type Module struct {
	catalogue *tags.Module
}

// New builds the unchecked removal over the catalogue it reads the Tag from.
func New(catalogue *tags.Module) *Module { return &Module{catalogue: catalogue} }

// Deletion is the plan of the removal of one Tag: the row DeletionIn read under the Workspace's
// lock (ADR 0108), so that the use case runs the Removal check (the Tag's Definition use) on the
// name it read before ApplyDeletion removes that row under the same lock.
type Deletion struct {
	tag *ent.Tag
}

// Name is the Tag's name, by which a Condition, a step or a Rule names it.
func (d *Deletion) Name() string { return d.tag.Name }

// DeletionIn opens the deletion of the Tag with the id in the transaction l holds the Workspace's
// lock in. A missing Tag is tags.ErrNotFound.
func (m *Module) DeletionIn(ctx context.Context, l *ent.WorkspaceLocked, id int64) (*Deletion, error) {
	tag, err := m.catalogue.Get(ctx, l.Scope(), id)
	if err != nil {
		return nil, err
	}
	return &Deletion{tag: tag}, nil
}

// ApplyDeletion removes the Tag d was opened on from the catalogue, and from every Contact that
// has it, without asking whether a Segment, an Automation, a Rule or a Broadcast names it: that is
// the use case's Removal check, run before in the transaction l holds the lock in, the one d was
// opened in.
func (m *Module) ApplyDeletion(ctx context.Context, l *ent.WorkspaceLocked, d *Deletion) error {
	err := l.Tag().DeleteOneID(d.tag.ID).Exec(ctx)
	if ent.IsNotFound(err) {
		return tags.ErrNotFound
	}
	return err
}
