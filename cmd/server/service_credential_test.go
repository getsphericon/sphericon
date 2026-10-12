package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/operator"
	"github.com/getsphericon/sphericon/ent"
)

var cliNow = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

func run(t *testing.T, ops serviceCredentialOps, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runServiceCredential(context.Background(), ops, func() time.Time { return cliNow }, args, &out)
	return out.String(), err
}

func issuing(err error) *serviceCredentialOpsMock {
	return &serviceCredentialOpsMock{
		IssueServiceCredentialFunc: func(_ context.Context, in operator.IssueInput) (*operator.Issued, error) {
			if err != nil {
				return nil, err
			}
			return &operator.Issued{ID: 7, Name: in.Name, Scopes: in.Scopes, ExpiresAt: in.ExpiresAt, Value: "omsc_value_once"}, nil
		},
	}
}

func TestServiceCredentialCreateShowsTheValueOnceWithItsNameAndScopes(t *testing.T) {
	ops := issuing(nil)

	out, err := run(t, ops, "create", "service-a", "usage:read,holds:write")

	require.NoError(t, err)
	calls := ops.IssueServiceCredentialCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "service-a", calls[0].In.Name)
	assert.Equal(t, []operator.Scope{operator.ScopeUsageRead, operator.ScopeHoldsWrite}, calls[0].In.Scopes)
	assert.Nil(t, calls[0].In.ExpiresAt)
	assert.Contains(t, out, "omsc_value_once")
	assert.Contains(t, out, "shown once")
}

func TestServiceCredentialCreateTakesAnOptionalExpiryFromNow(t *testing.T) {
	ops := issuing(nil)

	_, err := run(t, ops, "create", "--expires-in", "720h", "service-a", "usage:read")

	require.NoError(t, err)
	require.NotNil(t, ops.IssueServiceCredentialCalls()[0].In.ExpiresAt)
	assert.True(t, ops.IssueServiceCredentialCalls()[0].In.ExpiresAt.Equal(cliNow.Add(720*time.Hour)))
}

func TestServiceCredentialCreateExplainsARefusal(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{operator.ErrNotLicensed, "license"},
		{operator.ErrInvalidScope, "scope"},
		{operator.ErrInvalidCredential, "name"},
	} {
		_, err := run(t, issuing(tc.err), "create", "service-a", "usage:read")
		require.ErrorContains(t, err, tc.want)
	}
}

func TestServiceCredentialRotateShowsTheNewValueAndSaysTheOldOneStaysActive(t *testing.T) {
	ops := &serviceCredentialOpsMock{
		RotateServiceCredentialFunc: func(context.Context, int64, *time.Time) (*operator.Issued, error) {
			return &operator.Issued{ID: 9, Name: "service-a", Value: "omsc_rotated"}, nil
		},
	}

	out, err := run(t, ops, "rotate", "7")

	require.NoError(t, err)
	assert.EqualValues(t, 7, ops.RotateServiceCredentialCalls()[0].ID)
	assert.Contains(t, out, "omsc_rotated")
	assert.Contains(t, out, "revoke")
}

func TestServiceCredentialRevokeEndsTheCredential(t *testing.T) {
	ops := &serviceCredentialOpsMock{RevokeServiceCredentialFunc: func(context.Context, int64) error { return nil }}

	out, err := run(t, ops, "revoke", "7")

	require.NoError(t, err)
	assert.EqualValues(t, 7, ops.RevokeServiceCredentialCalls()[0].ID)
	assert.Contains(t, out, "revoked")
}

func TestServiceCredentialRevokeOfAMissingIdSaysSo(t *testing.T) {
	ops := &serviceCredentialOpsMock{RevokeServiceCredentialFunc: func(context.Context, int64) error { return operator.ErrCredentialNotFound }}

	_, err := run(t, ops, "revoke", "7")

	require.ErrorContains(t, err, "no service credential")
}

func TestServiceCredentialListShowsStateExpiryAndLastUsed(t *testing.T) {
	used := cliNow.Add(-time.Hour)
	soon := cliNow.Add(24 * time.Hour)
	past := cliNow.Add(-24 * time.Hour)
	ops := &serviceCredentialOpsMock{
		ServiceCredentialsFunc: func(context.Context, string) ([]*ent.ServiceCredential, error) {
			return []*ent.ServiceCredential{
				{ID: 1, Name: "service-a", Scopes: []string{"usage:read"}, LastUsedAt: &used, ExpiresAt: &soon},
				{ID: 2, Name: "service-a", Scopes: []string{"holds:write"}, RevokedAt: &past},
				{ID: 3, Name: "service-b", Scopes: []string{"usage:read"}, ExpiresAt: &past},
			}, nil
		},
	}

	out, err := run(t, ops, "list", "service-a")

	require.NoError(t, err)
	assert.Equal(t, "service-a", ops.ServiceCredentialsCalls()[0].Name)
	assert.Contains(t, out, "active")
	assert.Contains(t, out, "revoked")
	assert.Contains(t, out, "expired")
	assert.Contains(t, out, used.Format(time.RFC3339))
	assert.Contains(t, out, soon.Format(time.RFC3339))
	assert.Contains(t, out, "never")
}

func TestServiceCredentialNeedsAKnownCommandAndItsArguments(t *testing.T) {
	ops := issuing(nil)
	for _, args := range [][]string{nil, {"create"}, {"create", "service-a"}, {"rotate"}, {"rotate", "abc"}, {"revoke", "x"}, {"delete", "1"}, {"create", "--expires-in", "soon", "a", "usage:read"}} {
		_, err := run(t, ops, args...)
		require.Error(t, err, "args %v", args)
	}
	assert.Empty(t, ops.IssueServiceCredentialCalls())
}
