package contacterasure_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/contacterasure"
)

var apiRequester = contacterasure.Requester{Kind: contacterasure.RequesterAPIToken, ID: 1}

func TestAnErasureNamesExactlyOneIdentifier(t *testing.T) {
	env := testhelper.Setup(t)
	erasure := contacterasure.New(env.Bus, time.Now)
	acme := env.DB.Scoped(fixtures.AcmeID)

	neither := erasure.Erase(t.Context(), acme, contacterasure.Identifier{}, apiRequester)
	both := erasure.Erase(t.Context(), acme, contacterasure.Identifier{Email: new(fixtures.ContactAliceEmail), VisitorID: new("vis-1")}, apiRequester)
	idAndEmail := erasure.Erase(t.Context(), acme, contacterasure.Identifier{ContactID: fixtures.ContactAliceID, Email: new(fixtures.ContactAliceEmail)}, apiRequester)
	emptyVisitor := erasure.Erase(t.Context(), acme, contacterasure.Identifier{VisitorID: new("")}, apiRequester)

	require.ErrorIs(t, neither, contacterasure.ErrIdentifier)
	require.ErrorIs(t, both, contacterasure.ErrIdentifier)
	require.ErrorIs(t, idAndEmail, contacterasure.ErrIdentifier)
	require.ErrorIs(t, emptyVisitor, contacterasure.ErrIdentifier, "an empty visitor id names nothing")
}

func TestAnErasureByAnAddressThatIsNotOnePlainAddressIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	err := contacterasure.New(env.Bus, time.Now).Erase(t.Context(), env.DB.Scoped(fixtures.AcmeID),
		contacterasure.Identifier{Email: new("Alice <" + fixtures.ContactAliceEmail + ">")}, apiRequester)

	require.ErrorIs(t, err, contacterasure.ErrInvalidEmail)
	_, err = env.DB.Contact.Get(t.Context(), fixtures.ContactAliceID)
	require.NoError(t, err, "nothing is erased")
}
