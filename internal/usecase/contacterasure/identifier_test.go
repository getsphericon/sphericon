package contacterasure_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/contacterasure"
)

var apiRequester = contacterasure.Requester{Kind: contacterasure.RequesterAPIToken, ID: 1}

func TestAnErasureNamesExactlyOneIdentifier(t *testing.T) {
	env := testhelper.Setup(t)
	erasure := contacterasure.New(env.Bus)
	acme := env.DB.Scoped(fixtures.AcmeID)

	neither := erasure.Erase(t.Context(), acme, contacterasure.Identifier{}, apiRequester)
	both := erasure.Erase(t.Context(), acme, contacterasure.Identifier{Email: fixtures.ContactAliceEmail, VisitorID: "vis-1"}, apiRequester)
	idAndEmail := erasure.Erase(t.Context(), acme, contacterasure.Identifier{ContactID: fixtures.ContactAliceID, Email: fixtures.ContactAliceEmail}, apiRequester)

	require.ErrorIs(t, neither, contacterasure.ErrIdentifier)
	require.ErrorIs(t, both, contacterasure.ErrIdentifier)
	require.ErrorIs(t, idAndEmail, contacterasure.ErrIdentifier)
}

func TestAnErasureByAnAddressThatIsNotOnePlainAddressIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	err := contacterasure.New(env.Bus).Erase(t.Context(), env.DB.Scoped(fixtures.AcmeID),
		contacterasure.Identifier{Email: "Alice <" + fixtures.ContactAliceEmail + ">"}, apiRequester)

	require.ErrorIs(t, err, contacterasure.ErrInvalidEmail)
	_, err = env.DB.Contact.Get(t.Context(), fixtures.ContactAliceID)
	require.NoError(t, err, "nothing is erased")
}
