package workspaces

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
)

// CreateIn writes a new Workspace named name in tx, with the Workspace id publicID, a fresh
// collect key and the Send cap it starts with (ADR 0067; nil for none). A Workspace is reached
// through its Memberships, so the caller writes its Owner Membership on the same transaction
// (memberships.AddCreatorIn): the use case internal/usecase/workspacecreation, which refuses an
// Unverified User first (ADR 0030, ADR 0043). A unique violation (an id already taken) aborts
// tx, so the caller retries the whole transaction with a fresh id.
func CreateIn(ctx context.Context, tx *ent.Client, name, publicID string, sendCap *int) (*ent.Workspace, error) {
	collectKey, err := credentials.GenerateCollectKey()
	if err != nil {
		return nil, err
	}
	return tx.Workspace.Create().
		SetName(name).
		SetPublicID(publicID).
		SetCollectKey(collectKey).
		SetNillableSendCap(sendCap).
		Save(ctx)
}
