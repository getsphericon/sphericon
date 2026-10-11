package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mokevnin/sphericon/ee/operator"
)

func opsCreating(password string, err error) *operatorOpsMock {
	return &operatorOpsMock{
		CreateOperatorFunc:    func(context.Context, string) (string, error) { return password, err },
		ResetOperatorTOTPFunc: func(context.Context, string) error { return nil },
	}
}

func opsResetting(err error) *operatorOpsMock {
	return &operatorOpsMock{ResetOperatorTOTPFunc: func(context.Context, string) error { return err }}
}

func TestOperatorCreateShowsTheOneTimePassword(t *testing.T) {
	ops := opsCreating("s3cret-once", nil)
	var out bytes.Buffer

	require.NoError(t, runOperator(context.Background(), ops, []string{"create", "staff@sphericon.test"}, &out))

	calls := ops.CreateOperatorCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "staff@sphericon.test", calls[0].Email)
	assert.Contains(t, out.String(), "s3cret-once")
}

func TestOperatorCreateRefusesADuplicateEmail(t *testing.T) {
	err := runOperator(context.Background(), opsCreating("", operator.ErrDuplicate), []string{"create", "staff@sphericon.test"}, &bytes.Buffer{})

	require.ErrorContains(t, err, "already exists")
}

func TestOperatorCreateNeedsTheLicense(t *testing.T) {
	err := runOperator(context.Background(), opsCreating("", operator.ErrNotLicensed), []string{"create", "staff@sphericon.test"}, &bytes.Buffer{})

	require.ErrorContains(t, err, "license")
}

func TestOperatorNeedsACommandAndAnEmail(t *testing.T) {
	ops := opsCreating("x", nil)
	for _, args := range [][]string{nil, {"create"}, {"delete", "staff@sphericon.test"}} {
		require.Error(t, runOperator(context.Background(), ops, args, &bytes.Buffer{}), "args %v", args)
	}
	assert.Empty(t, ops.CreateOperatorCalls())
}

func TestOperatorResetTOTPClearsTheFactorAndSaysTheOperatorReenrols(t *testing.T) {
	ops := opsResetting(nil)
	var out bytes.Buffer

	require.NoError(t, runOperator(context.Background(), ops, []string{"reset-totp", "staff@sphericon.test"}, &out))

	calls := ops.ResetOperatorTOTPCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "staff@sphericon.test", calls[0].Email)
	assert.Contains(t, out.String(), "re-enrol")
}

func TestOperatorResetTOTPRefusesAnUnknownOperator(t *testing.T) {
	err := runOperator(context.Background(), opsResetting(operator.ErrNotFound), []string{"reset-totp", "nobody@sphericon.test"}, &bytes.Buffer{})

	require.ErrorContains(t, err, "no operator")
}

func TestOperatorResetTOTPNeedsTheLicense(t *testing.T) {
	err := runOperator(context.Background(), opsResetting(operator.ErrNotLicensed), []string{"reset-totp", "staff@sphericon.test"}, &bytes.Buffer{})

	require.ErrorContains(t, err, "license")
}
