package consentrecords_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// ADR 0062: accepting what the screen showed answers with what is pending after it, nothing once
// every pending acceptance is accepted at its current version.
func TestAcceptingThePendingAcceptancesAnswersWithNothingLeftPending(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	before, err := testhelper.Module[*consentrecords.Module](env).Pending(t.Context(), fixtures.MemberMaryID)
	require.NoError(t, err)
	require.Len(t, before, 1)

	left, err := testhelper.Module[*consentrecords.Module](env).AcceptPending(t.Context(), fixtures.MemberMaryID,
		[]consentrecords.ReAcceptance{{Key: fixtures.ServiceTermsKey, Version: fixtures.ServiceTermsCurrentLabel}})

	require.NoError(t, err)
	assert.Empty(t, left)
}
