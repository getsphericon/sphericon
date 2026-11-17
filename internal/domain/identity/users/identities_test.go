package users_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// ADR 0176: every link tells the owner, as a link at sign-in does: the notice with the link that
// undoes it.
func TestLinkingAProviderAccountMailsTheOwnerTheNotice(t *testing.T) {
	env := testhelper.Setup(t)

	link, err := testhelper.Module[*users.Module](env).LinkIdentity(t.Context(), users.LinkIdentityInput{UserID: fixtures.MemberMaryID, Provider: "google", Subject: "google-subject-of-mary-at-work"})

	require.NoError(t, err)
	assert.True(t, link.Added)
	sent := env.SystemMail.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, fixtures.MemberMaryEmail, sent[0].To)
	assert.Contains(t, sent[0].Subject, "sign-in provider was linked")
	assert.Contains(t, sent[0].Text, "/signin-not-me?token=")
}

// A provider account the User holds already is no new way to sign in: nobody is told.
func TestLinkingAProviderAccountTheUserHoldsMailsNothing(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := testhelper.Module[*users.Module](env).LinkIdentity(t.Context(), users.LinkIdentityInput{UserID: fixtures.MemberMaryID, Provider: fixtures.MaryGoogleIdentityProvider, Subject: fixtures.MaryGoogleIdentitySubject})

	require.NoError(t, err)
	assert.Empty(t, env.SystemMail.Messages())
}
