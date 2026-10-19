package main

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/staff"
	"github.com/getsphericon/sphericon/ent/servicecredential"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// run executes `sphericon service-credential <args>` against the real Staff module of env; the
// command clock is the real one because the module compares expiries with it.
func run(t *testing.T, env *testhelper.TestEnv, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := executeAt(time.Now(), staffCLI{env.StaffMembers}, &out, append([]string{"service-credential"}, args...)...)
	return out.String(), err
}

func credentials(t *testing.T, env *testhelper.TestEnv, name string) []*staff.Issued {
	t.Helper()
	rows, err := env.StaffMembers.ServiceCredentials(context.Background(), name)
	require.NoError(t, err)
	issued := make([]*staff.Issued, 0, len(rows))
	for _, row := range rows {
		issued = append(issued, &staff.Issued{ID: row.ID, Name: row.Name, ExpiresAt: row.ExpiresAt})
	}
	return issued
}

func TestServiceCredentialCreateShowsTheValueOnceWithItsNameAndScopes(t *testing.T) {
	env := testhelper.Setup(t)

	out, err := run(t, env, "create", "svc-new", "usage:read,holds:write")

	require.NoError(t, err)
	row := env.DB.ServiceCredential.Query().Where(servicecredential.Name("svc-new")).OnlyX(context.Background())
	assert.ElementsMatch(t, []string{"usage:read", "holds:write"}, row.Scopes)
	assert.Nil(t, row.ExpiresAt)
	assert.Contains(t, out, "smsc_")
	assert.Contains(t, out, "shown once")
}

func TestServiceCredentialCreateTakesAnOptionalExpiryFromNow(t *testing.T) {
	env := testhelper.Setup(t)
	before := time.Now()

	_, err := run(t, env, "create", "--expires-in", "720h", "svc-new", "usage:read")

	require.NoError(t, err)
	row := env.DB.ServiceCredential.Query().Where(servicecredential.Name("svc-new")).OnlyX(context.Background())
	require.NotNil(t, row.ExpiresAt)
	assert.WithinDuration(t, before.Add(720*time.Hour), *row.ExpiresAt, time.Minute)
}

func TestServiceCredentialCreateTakesTheExpiryAfterTheArguments(t *testing.T) {
	env := testhelper.Setup(t)
	before := time.Now()

	_, err := run(t, env, "create", "svc-new", "usage:read", "--expires-in", "720h")

	require.NoError(t, err)
	row := env.DB.ServiceCredential.Query().Where(servicecredential.Name("svc-new")).OnlyX(context.Background())
	require.NotNil(t, row.ExpiresAt)
	assert.WithinDuration(t, before.Add(720*time.Hour), *row.ExpiresAt, time.Minute)
}

func TestServiceCredentialCreateExplainsARefusal(t *testing.T) {
	licensed := testhelper.Setup(t)
	before := licensed.DB.ServiceCredential.Query().CountX(context.Background())
	unlicensed := testhelper.Setup(t, testhelper.WithoutLicense())
	for _, tc := range []struct {
		env  *testhelper.TestEnv
		args []string
		want string
	}{
		{unlicensed, []string{"create", "svc-new", "usage:read"}, "license"},
		{licensed, []string{"create", "svc-new", "no:such-scope"}, "scope"},
		{licensed, []string{"create", " ", "usage:read"}, "name"},
	} {
		_, err := run(t, tc.env, tc.args...)
		require.ErrorContains(t, err, tc.want)
	}
	assert.Equal(t, before, licensed.DB.ServiceCredential.Query().CountX(context.Background()), "a refusal creates nothing")
}

func TestServiceCredentialRotateShowsTheNewValueAndSaysTheOldOneStaysActive(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := run(t, env, "create", "svc-new", "usage:read")
	require.NoError(t, err)
	old := credentials(t, env, "svc-new")[0]
	revokedBefore := env.DB.ServiceCredential.Query().Where(servicecredential.RevokedAtNotNil()).CountX(context.Background())

	out, err := run(t, env, "rotate", itoa(old.ID))

	require.NoError(t, err)
	assert.Len(t, credentials(t, env, "svc-new"), 2, "a second credential with the same name")
	assert.Equal(t, revokedBefore, env.DB.ServiceCredential.Query().Where(servicecredential.RevokedAtNotNil()).CountX(context.Background()), "the old one stays active")
	assert.Contains(t, out, "smsc_")
	assert.Contains(t, out, "revoke")
}

func TestServiceCredentialRevokeEndsTheCredential(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := run(t, env, "create", "svc-new", "usage:read")
	require.NoError(t, err)
	id := credentials(t, env, "svc-new")[0].ID

	out, err := run(t, env, "revoke", itoa(id))

	require.NoError(t, err)
	assert.NotNil(t, env.DB.ServiceCredential.GetX(context.Background(), id).RevokedAt)
	assert.Contains(t, out, "revoked")
}

func TestServiceCredentialRevokeOfAMissingIdSaysSo(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := run(t, env, "revoke", "999999")

	require.ErrorContains(t, err, "no service credential")
}

func TestServiceCredentialListShowsStateExpiryAndLastUsed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	_, err := run(t, env, "create", "--expires-in", "24h", "svc-new", "usage:read")
	require.NoError(t, err)
	_, err = run(t, env, "create", "svc-new", "holds:write")
	require.NoError(t, err)
	revoked := env.DB.ServiceCredential.Query().Where(servicecredential.Name("svc-new"), servicecredential.ExpiresAtIsNil()).OnlyX(ctx)
	_, err = run(t, env, "revoke", itoa(revoked.ID))
	require.NoError(t, err)

	out, err := run(t, env, "list", "svc-new")

	require.NoError(t, err)
	assert.Contains(t, out, "active")
	assert.Contains(t, out, "revoked")
	assert.Contains(t, out, "never")
}

func TestServiceCredentialRefusesExtraArgumentsAndANegativeExpiry(t *testing.T) {
	env := testhelper.Setup(t)
	before := env.DB.ServiceCredential.Query().CountX(context.Background())

	_, err := run(t, env, "revoke", "7", "8")
	require.Error(t, err)
	_, err = run(t, env, "create", "--expires-in=-1h", "svc-new", "usage:read")
	require.Error(t, err)
	assert.Equal(t, before, env.DB.ServiceCredential.Query().CountX(context.Background()))
}

func TestServiceCredentialNeedsAKnownCommandAndItsArguments(t *testing.T) {
	env := testhelper.Setup(t)
	before := env.DB.ServiceCredential.Query().CountX(context.Background())
	for _, args := range [][]string{nil, {"create"}, {"create", "svc-new"}, {"rotate"}, {"rotate", "abc"}, {"revoke", "x"}, {"delete", "1"}, {"create", "--expires-in", "soon", "a", "usage:read"}} {
		_, err := run(t, env, args...)
		require.Error(t, err, "args %v", args)
	}
	assert.Equal(t, before, env.DB.ServiceCredential.Query().CountX(context.Background()))
}

func itoa(id int64) string { return strconv.FormatInt(id, 10) }
