// Package tagsunchecked holds the tags module's unchecked removal (ADR 0110, ADR 0145): deleting a
// Tag asks whether a Segment, an Automation, a Rule or a Broadcast names it, modules above tags, so
// that check is the use case internal/usecase/tagdeletion's, the one kind of package easement lets
// import this one. Every rule of the Tag's own row stays here. The tags module does not import it.
package tagsunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
)

// Deletion is the removal of one Tag, opened on its row under the Workspace's lock (ADR 0108) so
// that the use case runs the Removal check (the Tag's Definition use) between opening and applying
// it, on the name the deletion read.
type Deletion struct {
	l   *ent.WorkspaceLocked
	tag *ent.Tag
}

// DeletionIn opens the deletion of the Tag with the id in the transaction l holds the Workspace's
// lock in. A missing Tag is tags.ErrNotFound.
func DeletionIn(ctx context.Context, l *ent.WorkspaceLocked, id int64) (*Deletion, error) {
	tag, err := tags.Get(ctx, l.Scope(), id)
	if err != nil {
		return nil, err
	}
	return &Deletion{l: l, tag: tag}, nil
}

// Name is the Tag's name, by which a Condition, a step or a Rule names it.
func (d *Deletion) Name() string { return d.tag.Name }

// Apply removes the Tag from the catalogue, and from every Contact that has it, without asking
// whether a Segment, an Automation, a Rule or a Broadcast names it: that is the use case's Removal
// check, run under the same lock before.
func (d *Deletion) Apply(ctx context.Context) error {
	err := d.l.Tag().DeleteOneID(d.tag.ID).Exec(ctx)
	if ent.IsNotFound(err) {
		return tags.ErrNotFound
	}
	return err
}
