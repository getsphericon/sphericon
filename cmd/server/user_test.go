package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeUserOps struct {
	reset   []string
	changed bool
	created [][2]string
}

func (f *fakeUserOps) CreateOwner(_ context.Context, name, email string) (string, error) {
	f.created = append(f.created, [2]string{name, email})
	return "one-time-pw", nil
}

func TestUserCreateOwnerPrintsTheOneTimePassword(t *testing.T) {
	ops := &fakeUserOps{}
	var out bytes.Buffer

	require.NoError(t, execute(ops, &out, "user", "create-owner", "ada@example.com", "Ada", "Lovelace"))

	assert.Equal(t, [][2]string{{"Ada Lovelace", "ada@example.com"}}, ops.created)
	assert.Contains(t, out.String(), "one-time-pw")
}

func (f *fakeUserOps) ResetSecondFactor(_ context.Context, email string) (bool, error) {
	f.reset = append(f.reset, email)
	return f.changed, nil
}

func TestUserResetSecondFactorByEmail(t *testing.T) {
	ops := &fakeUserOps{changed: true}
	var out bytes.Buffer

	require.NoError(t, execute(ops, &out, "user", "reset-second-factor", "sam@globex.test"))

	assert.Equal(t, []string{"sam@globex.test"}, ops.reset)
	assert.Contains(t, out.String(), "reset")
}

func TestUserResetSecondFactorReportsNoChange(t *testing.T) {
	ops := &fakeUserOps{changed: false}
	var out bytes.Buffer

	require.NoError(t, execute(ops, &out, "user", "reset-second-factor", "john@acme.test"))

	assert.Contains(t, out.String(), "nothing changed")
}

func TestUserCommandNeedsAKnownSubcommandAndAnEmail(t *testing.T) {
	ops := &fakeUserOps{}
	for _, args := range [][]string{nil, {"reset-second-factor"}, {"delete", "sam@globex.test"}} {
		require.Error(t, execute(ops, &bytes.Buffer{}, append([]string{"user"}, args...)...), "args %v", args)
	}
	assert.Empty(t, ops.reset)
}
