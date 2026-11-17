package workspaces_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestIDByPublicIDIn(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*workspaces.Module](env)

	id, err := m.IDByPublicIDIn(context.Background(), env.DB, fixtures.AcmePublicID)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmeID, id)
	_, err = m.IDByPublicIDIn(context.Background(), env.DB, "NoSuchId")
	require.Error(t, err)
	_, err = m.IDByPublicIDIn(context.Background(), env.DB, "acmews01")
	require.Error(t, err, "the id is case-sensitive")
}

func TestTheOldestLiveWorkspaceIsNeverADeletedOne(t *testing.T) {
	env := testhelper.Setup(t)
	a := testhelper.Module[*workspaces.Module](env)
	oldest, err := a.OldestLiveID(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, fixtures.AcmeID, oldest)

	env.DeleteWorkspace(t, fixtures.AcmeID, time.Now())

	next, err := a.OldestLiveID(t.Context())
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.GlobexID, next)
}
