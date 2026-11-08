package staffmembers_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The email of a Staff member is taken: staff_members_email_key refuses a second one with
// ErrDuplicate. The refusal ends the test: on the pool, outside any transaction, the violation
// aborts the test's own transaction, as it would abort none in production.
func TestCreatingAStaffMemberWithATakenEmailIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.StaffMembers.Create(t.Context(), fixtures.StaffEnrolledEmail)

	require.ErrorIs(t, err, staffmembers.ErrDuplicate)
}

func TestTheTakenEmailIsComparedNormalized(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.StaffMembers.Create(t.Context(), " "+fixtures.StaffFreshEmail+" ")

	require.ErrorIs(t, err, staffmembers.ErrDuplicate)
}
