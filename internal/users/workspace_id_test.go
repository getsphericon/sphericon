package users_test

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/workspaceid"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/users"
)

func TestCreateWorkspaceGivesItAnId(t *testing.T) {
	env := testhelper.Setup(t)

	ws, err := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords).CreateWorkspace(t.Context(), fixtures.MemberMaryID, "Fresh")
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

	ws, err := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, users.WithWorkspaceIDs(generate)).
		CreateWorkspace(t.Context(), fixtures.MemberMaryID, "Fresh")
	require.NoError(t, err)

	assert.Equal(t, "FreshId1", ws.PublicID)
	assert.EqualValues(t, 2, calls.Load())
}
