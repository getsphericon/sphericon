package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/staffmember"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const staffEmail = "new-staff@sphericon.test"

func TestStaffCreateMakesTheStaffMemberAndShowsTheOneTimePasswordOnce(t *testing.T) {
	env := testhelper.Setup(t)
	var out bytes.Buffer

	require.NoError(t, execute(staffCLI{env.StaffMembers}, &out, "staff", "create", staffEmail))

	member := env.DB.StaffMember.Query().Where(staffmember.Email(staffEmail)).OnlyX(context.Background())
	assert.NotEmpty(t, member.PasswordHash, "the Staff member can sign in")
	assert.NotContains(t, out.String(), member.PasswordHash, "only the password is shown, never its hash")
	assert.Contains(t, out.String(), "shown once")
}

func TestStaffCreateRefusesADuplicateEmail(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, execute(staffCLI{env.StaffMembers}, &bytes.Buffer{}, "staff", "create", staffEmail))

	err := execute(staffCLI{env.StaffMembers}, &bytes.Buffer{}, "staff", "create", staffEmail)

	require.ErrorContains(t, err, "already exists")
}

func TestStaffCreateNeedsTheLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())

	err := execute(staffCLI{env.StaffMembers}, &bytes.Buffer{}, "staff", "create", staffEmail)

	require.ErrorContains(t, err, "license")
	assert.False(t, env.DB.StaffMember.Query().Where(staffmember.Email(staffEmail)).ExistX(context.Background()))
}

func TestStaffNeedsACommandAndAnEmail(t *testing.T) {
	env := testhelper.Setup(t)
	for _, args := range [][]string{nil, {"create"}, {"delete", staffEmail}} {
		require.Error(t, execute(staffCLI{env.StaffMembers}, &bytes.Buffer{}, append([]string{"staff"}, args...)...), "args %v", args)
	}
	assert.False(t, env.DB.StaffMember.Query().Where(staffmember.Email(staffEmail)).ExistX(context.Background()))
}

func TestStaffResetTOTPClearsTheFactorAndSaysTheStaffReenrols(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	_, err := env.StaffMembers.Create(ctx, staffEmail)
	require.NoError(t, err)
	enrolled := env.DB.StaffMember.Query().Where(staffmember.Email(staffEmail)).OnlyX(ctx)
	env.DB.StaffMember.UpdateOneID(enrolled.ID).SetTotpSecretEncrypted("enrolled").ExecX(ctx)
	var out bytes.Buffer

	require.NoError(t, execute(staffCLI{env.StaffMembers}, &out, "staff", "reset-totp", staffEmail))

	after := env.DB.StaffMember.GetX(ctx, enrolled.ID)
	assert.Empty(t, after.TotpSecretEncrypted, "the factor is cleared")
	assert.Greater(t, after.SessionEpoch, enrolled.SessionEpoch, "its sessions end")
	assert.Contains(t, out.String(), "re-enrol")
}

func TestStaffResetTOTPRefusesAnUnknownStaffMember(t *testing.T) {
	env := testhelper.Setup(t)

	err := execute(staffCLI{env.StaffMembers}, &bytes.Buffer{}, "staff", "reset-totp", "nobody@sphericon.test")

	require.ErrorContains(t, err, "no staff")
}

func TestStaffResetTOTPNeedsTheLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())

	err := execute(staffCLI{env.StaffMembers}, &bytes.Buffer{}, "staff", "reset-totp", staffEmail)

	require.ErrorContains(t, err, "license")
}
