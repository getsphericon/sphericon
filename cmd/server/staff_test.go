package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/staff"
)

func opsCreating(password string, err error) *staffOpsMock {
	return &staffOpsMock{
		CreateStaffMemberFunc: func(context.Context, string) (string, error) { return password, err },
		ResetStaffTOTPFunc:    func(context.Context, string) error { return nil },
	}
}

func opsResetting(err error) *staffOpsMock {
	return &staffOpsMock{ResetStaffTOTPFunc: func(context.Context, string) error { return err }}
}

func TestStaffCreateShowsTheOneTimePassword(t *testing.T) {
	ops := opsCreating("s3cret-once", nil)
	var out bytes.Buffer

	require.NoError(t, execute(ops, &out, "staff", "create", "staff@sphericon.test"))

	calls := ops.CreateStaffMemberCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "staff@sphericon.test", calls[0].Email)
	assert.Contains(t, out.String(), "s3cret-once")
}

func TestStaffCreateRefusesADuplicateEmail(t *testing.T) {
	err := execute(opsCreating("", staff.ErrDuplicate), &bytes.Buffer{}, "staff", "create", "staff@sphericon.test")

	require.ErrorContains(t, err, "already exists")
}

func TestStaffCreateNeedsTheLicense(t *testing.T) {
	err := execute(opsCreating("", staff.ErrNotLicensed), &bytes.Buffer{}, "staff", "create", "staff@sphericon.test")

	require.ErrorContains(t, err, "license")
}

func TestStaffNeedsACommandAndAnEmail(t *testing.T) {
	ops := opsCreating("x", nil)
	for _, args := range [][]string{nil, {"create"}, {"delete", "staff@sphericon.test"}} {
		require.Error(t, execute(ops, &bytes.Buffer{}, append([]string{"staff"}, args...)...), "args %v", args)
	}
	assert.Empty(t, ops.CreateStaffMemberCalls())
}

func TestStaffResetTOTPClearsTheFactorAndSaysTheStaffReenrols(t *testing.T) {
	ops := opsResetting(nil)
	var out bytes.Buffer

	require.NoError(t, execute(ops, &out, "staff", "reset-totp", "staff@sphericon.test"))

	calls := ops.ResetStaffTOTPCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "staff@sphericon.test", calls[0].Email)
	assert.Contains(t, out.String(), "re-enrol")
}

func TestStaffResetTOTPRefusesAnUnknownStaffMember(t *testing.T) {
	err := execute(opsResetting(staff.ErrNotFound), &bytes.Buffer{}, "staff", "reset-totp", "nobody@sphericon.test")

	require.ErrorContains(t, err, "no staff")
}

func TestStaffResetTOTPNeedsTheLicense(t *testing.T) {
	err := execute(opsResetting(staff.ErrNotLicensed), &bytes.Buffer{}, "staff", "reset-totp", "staff@sphericon.test")

	require.ErrorContains(t, err, "license")
}
