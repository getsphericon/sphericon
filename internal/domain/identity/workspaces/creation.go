package workspaces

import (
	"context"
	"errors"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
)

// errPublicIDsTaken is a generator of Workspace ids that drew only taken ones, publicIDAttempts in
// a row: one taken id is rare already (workspaceid.New), so the generator is broken.
var errPublicIDsTaken = errors.New("workspaces: every workspace id drawn is taken")

// publicIDAttempts is how many Workspace ids CreateIn draws before it gives up.
const publicIDAttempts = 3

// The unique constraints of the Workspaces (ADR 0106): no client can break them.
var _ = db.Declare(
	db.Fault("workspaces_public_id_key",
		"a Workspace is inserted ON CONFLICT (public_id) DO NOTHING, and a taken id is drawn again (CreateIn)"),
	db.Fault("workspaces_collect_key_key", "the collect key is drawn at random (credentials.GenerateCollectKey)"),
)

// CreateIn writes a new Workspace named name in tx, with a Workspace id newPublicID draws (again
// when the one drawn is taken, a fault after publicIDAttempts draws), a fresh collect key and the
// Send cap it starts with (ADR 0067; nil for none). A Workspace is reached through its Memberships,
// so the caller writes its Owner Membership on the same transaction (memberships.AddCreatorIn): the
// use cases internal/usecase/workspacecreation and internal/usecase/signup, which refuse an
// Unverified User first (ADR 0030, ADR 0043).
func CreateIn(ctx context.Context, tx *ent.Client, name string, newPublicID func() string, sendCap *int) (*ent.Workspace, error) {
	collectKey, err := credentials.GenerateCollectKey()
	if err != nil {
		return nil, err
	}
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
			return tx.Workspace.Get(ctx, id)
		}
	}
	return nil, errPublicIDsTaken
}
