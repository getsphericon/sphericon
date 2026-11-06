package workspaces_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/workspaces"
)

func TestIDByPublicID(t *testing.T) {
	env := testhelper.Setup(t)

	id, err := workspaces.IDByPublicID(context.Background(), env.DB, fixtures.AcmePublicID)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmeID, id)
	_, err = workspaces.IDByPublicID(context.Background(), env.DB, "NoSuchId")
	require.Error(t, err)
	_, err = workspaces.IDByPublicID(context.Background(), env.DB, "acmews01")
	require.Error(t, err, "the id is case-sensitive")
}

func TestBootstrapScopeNeverTargetsADeletedWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	a := workspaces.New(env.DB, env.Bus)
	oldest, err := a.BootstrapScope(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, fixtures.AcmeID, oldest.WorkspaceID())

	env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(time.Now()).ExecX(t.Context())

	next, err := a.BootstrapScope(t.Context())
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.GlobexID, next.WorkspaceID())
}
