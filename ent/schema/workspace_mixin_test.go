package schema_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/schema"
)

// A workspace-scoped entity gets workspace_id and a required, unique
// workspace edge back to the named reverse edge on Workspace (ADR 0017).
func TestWorkspaceMixinScopesAnEntityToItsWorkspace(t *testing.T) {
	m := schema.WorkspaceMixin{Ref: "contacts"}

	fields := m.Fields()
	require.Len(t, fields, 1)
	require.Equal(t, "workspace_id", fields[0].Descriptor().Name)

	edges := m.Edges()
	require.Len(t, edges, 1)
	d := edges[0].Descriptor()
	require.Equal(t, "workspace", d.Name)
	require.Equal(t, "contacts", d.RefName)
	require.Equal(t, "workspace_id", d.Field)
	require.True(t, d.Required)
	require.True(t, d.Unique)
	require.True(t, d.Inverse)
}
