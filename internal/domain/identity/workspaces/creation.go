package workspaces

import (
	"context"
	"errors"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/ent/workspacestanding"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The unique constraints of the Workspaces (ADR 0106): no client can break them.
var _ = db.Declare(
	db.Fault("workspaces_public_id_key",
		"a Workspace is inserted ON CONFLICT (public_id) DO NOTHING, and a taken id is drawn again (CreateIn)"),
	db.Fault("workspaces_collect_key_key", "the collect key is drawn at random (credentials.GenerateCollectKey)"),
	db.Fault("workspace_standing_workspace_id_key", "a standing is written ON CONFLICT (workspace_id), by CreateIn and the feed"),
)

// errPublicIDsTaken is a generator of Workspace ids that drew only taken ones, publicIDAttempts in
// a row: one taken id is rare already (workspaceid.New), so the generator is broken.
var errPublicIDsTaken = errors.New("workspaces: every workspace id drawn is taken")

// publicIDAttempts is how many Workspace ids CreateIn draws before it gives up.
const publicIDAttempts = 3

// CreateIn writes a new Workspace named name in tx, with a Workspace id newPublicID draws (again
// when the one drawn is taken, a fault after publicIDAttempts draws), a fresh collect key and the
// Send cap it starts with (ADR 0067; nil for none), and its standing beside it (ADR 0168). Only the creation of a Workspace with its Owner
// calls it (memberships.CreateWorkspaceIn): a Workspace is reached through its Memberships, so one
// without its Owner is unreachable.
func CreateIn(ctx context.Context, tx *ent.Client, name string, newPublicID func() string, sendCap *int) (*ent.Workspace, error) {
	name, err := validation.Name(name)
	if err != nil {
		return nil, err
	}
	if err := CheckSendCap(sendCap); err != nil {
		return nil, err
	}
	collectKey := credentials.GenerateCollectKey()
	for range publicIDAttempts {
		id, err := tx.Workspace.Create().
			SetName(name).
			SetPublicID(newPublicID()).
			SetCollectKey(collectKey).
			SetNillableSendCap(sendCap).
			OnConflict(entsql.ConflictColumns(workspace.FieldPublicID)).
			DoNothing().
			ID(ctx)
		inserted, err := db.Inserted(err)
		switch {
		case err != nil:
			return nil, err
		case inserted:
			// What the service keeps about it for itself lives beside it (ADR 0168); the feed's
			// hook may have made it already, with the Workspace's first feed position.
			err := tx.WorkspaceStanding.Create().SetWorkspaceID(id).
				OnConflictColumns(workspacestanding.FieldWorkspaceID).DoNothing().Exec(ctx)
			if _, err := db.Inserted(err); err != nil {
				return nil, err
			}
			return tx.Workspace.Get(ctx, id)
		}
	}
	return nil, errPublicIDsTaken
}
