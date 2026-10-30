package accounts_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/workspaceid"
)

func TestCreateWorkspaceGivesItAnId(t *testing.T) {
	env := testhelper.Setup(t)

	ws, err := accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords).CreateWorkspace(t.Context(), fixtures.MemberMaryID, "Fresh")
	require.NoError(t, err)

	assert.Regexp(t, workspaceid.Pattern, ws.PublicID)
}

func TestCreateWorkspaceDrawsAnotherIdWhenOneIsTaken(t *testing.T) {
	env := testhelper.Setup(t)
	var calls atomic.Int32
	generate := func() string {
		if calls.Add(1) == 1 {
			return fixtures.AcmePublicID
		}
		return "FreshId1"
	}

	ws, err := accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, accounts.WithWorkspaceIDs(generate)).
		CreateWorkspace(t.Context(), fixtures.MemberMaryID, "Fresh")
	require.NoError(t, err)

	assert.Equal(t, "FreshId1", ws.PublicID)
	assert.EqualValues(t, 2, calls.Load())
}

func TestWorkspaceIDByPublicID(t *testing.T) {
	env := testhelper.Setup(t)

	id, err := accounts.WorkspaceIDByPublicID(context.Background(), env.DB, fixtures.AcmePublicID)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmeID, id)
	_, err = accounts.WorkspaceIDByPublicID(context.Background(), env.DB, "NoSuchId")
	require.Error(t, err)
	_, err = accounts.WorkspaceIDByPublicID(context.Background(), env.DB, "acmews01")
	require.Error(t, err, "the id is case-sensitive")
}

func TestBootstrapScopeNeverTargetsADeletedWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	a := accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords)
	oldest, err := a.BootstrapScope(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, fixtures.AcmeID, oldest.WorkspaceID())

	env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(time.Now()).ExecX(t.Context())

	next, err := a.BootstrapScope(t.Context())
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.GlobexID, next.WorkspaceID())
}
