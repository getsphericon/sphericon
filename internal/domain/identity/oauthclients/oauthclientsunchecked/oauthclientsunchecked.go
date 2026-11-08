// Package oauthclientsunchecked holds the oauthclients module's unchecked removal (ADR 0110): the
// authorization codes that name a Workspace go only in the last transaction of its erase, after
// every row the Workspace owned, so the check is the use case internal/usecase/workspacedeletion's
// (ADR 0043); only a use case may import this package. The oauthclients module does not import it.
package oauthclientsunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/oauthcode"
)

// DeleteCodesIn deletes every authorization code that names the Workspace, on the transaction
// tx: the last step of erasing a Workspace (ADR 0043), whose codes are not Workspace-scoped rows.
func DeleteCodesIn(ctx context.Context, tx *ent.Client, workspaceID int64) error {
	_, err := tx.OAuthCode.Delete().Where(oauthcode.WorkspaceID(workspaceID)).Exec(ctx)
	return err
}
