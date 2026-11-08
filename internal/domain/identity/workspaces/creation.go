package workspaces

import "github.com/getsphericon/sphericon/internal/infra/db"

// The unique constraints of the Workspaces (ADR 0106): no client can break them. A Workspace is
// created by workspacesunchecked.CreateIn, which this module's subpackage holds (ADR 0110).
var _ = db.Declare(
	db.Fault("workspaces_public_id_key",
		"a Workspace is inserted ON CONFLICT (public_id) DO NOTHING, and a taken id is drawn again (workspacesunchecked.CreateIn)"),
	db.Fault("workspaces_collect_key_key", "the collect key is drawn at random (credentials.GenerateCollectKey)"),
)
