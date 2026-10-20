package accounts_test

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/workspaceid"
)

func TestCreateWorkspaceGivesItAnId(t *testing.T) {
	env := testhelper.Setup(t)

	ws, err := accounts.New(env.DB, env.Bus).CreateWorkspace(t.Context(), fixtures.MemberMaryID, "Fresh")
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

	ws, err := accounts.New(env.DB, env.Bus, accounts.WithWorkspaceIDs(generate)).
		CreateWorkspace(t.Context(), fixtures.MemberMaryID, "Fresh")
	require.NoError(t, err)

	assert.Equal(t, "FreshId1", ws.PublicID)
	assert.EqualValues(t, 2, calls.Load())
}
