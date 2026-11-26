package external_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/confirmation"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/sending/confirmationrequests"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// confirmToken is the token of the confirmation link in a message, as its Confirm button carries it.
var confirmToken = regexp.MustCompile(`/e/confirm/([A-Za-z0-9_\-.]+)`)

// requester is a client that may ask for confirmation emails.
func requester(t *testing.T, env *testhelper.TestEnv) *externalapi.Client {
	t.Helper()
	return env.ExternalScoped(t, apitokens.ScopeConfirmationsSend, apitokens.ScopeContactsWrite)
}

func askConfirmation(t *testing.T, c *externalapi.Client, contactID int64) (externalapi.ConfirmationRequestsCreateRes, error) {
	t.Helper()
	return c.ConfirmationRequestsCreate(t.Context(), externalapi.ConfirmationRequestsCreateParams{ContactId: externalapi.EntityId(contactID)})
}

// mailTo is the messages the provider took for the address.
func mailTo(env *testhelper.TestEnv, address string) []messaging.EmailMessage {
	return lo.Filter(env.CustomerMail.Messages(), func(m messaging.EmailMessage, _ int) bool { return m.To == address })
}

// An unconfirmed Contact of a Workspace that requires confirmed opt-in gets one confirmation email
// from the request; following its link and pressing Confirm records a Confirmation of the double
// opt-in, and the next Broadcast reaches them (ADR 0013).
func TestAConfirmationRequestMakesAnUnconfirmedContactMailable(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.SetUpAcmeConfirmationEmail(t)
	_, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWorkspacesSetConfirmedOptIn(ctx,
		&siteapi.SiteSetConfirmedOptInInput{Revision: siteapi.Revision(env.WorkspaceRevision(ctx, t, string(fixtures.AcmePublicID))), Enabled: true},
		siteapi.SiteWorkspacesSetConfirmedOptInParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	c := requester(t, env)
	const address = "late.reader@example.com"
	created, err := c.ContactsCreate(ctx, &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress(address)})
	require.NoError(t, err)
	contactID := int64(created.ID)
	audience := func() []int64 {
		s := env.DB.Scoped(fixtures.AcmeID)
		p, err := eligibility.New().BroadcastAudience(ctx, s, channels.Email, nil)
		require.NoError(t, err)
		return s.Contact().Query().Where(p).IDsX(ctx)
	}
	before := audience()
	require.NotContains(t, before, contactID, "unconfirmed")

	res, err := askConfirmation(t, c, contactID)
	require.NoError(t, err)
	assert.Equal(t, &externalapi.ConfirmationRequestsCreateAccepted{Status: externalapi.ConfirmationRequestStatusAccepted}, res)
	env.PerformEnqueuedJobs(t, confirmationrequests.SendArgs{})

	mail := mailTo(env, address)
	require.Len(t, mail, 1, "one confirmation email")
	assert.Equal(t, testhelper.AcmeConfirmationFrom, mail[0].From)
	link := confirmToken.FindStringSubmatch(mail[0].HTML)
	require.Len(t, link, 2)
	err = env.SiteAnonymous(t).SitePublicConfirmationsPerform(ctx, siteapi.SitePublicConfirmationsPerformParams{Token: link[1]})
	require.NoError(t, err)

	got := env.DB.Confirmation.Query().Where(confirmation.WorkspaceID(fixtures.AcmeID), confirmation.DestinationEQ(address)).OnlyX(ctx)
	after := audience()
	assert.Equal(t, confirmation.ProvenanceDoubleOptIn, got.Provenance)
	assert.Contains(t, after, contactID, "the next Broadcast reaches them")
}

// An address that holds a Confirmation gets no email, and the answer says it is confirmed.
func TestAConfirmationRequestForAConfirmedAddressSendsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	env.SetUpAcmeConfirmationEmail(t)

	res, err := askConfirmation(t, requester(t, env), fixtures.ContactLiamID)
	require.NoError(t, err)
	queued := env.JobRowsOf(t, confirmationrequests.SendArgs{}.Kind())
	assert.Equal(t, &externalapi.ConfirmationRequestsCreateOK{Status: externalapi.ConfirmationRequestStatusConfirmed}, res)
	assert.Empty(t, queued)
}

// The Workspace's request is refused for an address that opted out of everything or is suppressed,
// naming which, and while no confirmation email is set up; none queues an email.
func TestAConfirmationRequestIsRefused(t *testing.T) {
	cases := map[string]struct {
		before func(t *testing.T, env *testhelper.TestEnv)
		detail string
	}{
		"no confirmation email": {func(*testing.T, *testhelper.TestEnv) {}, "confirmation email"},
		"opted out of everything": {func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			env.SetUpAcmeConfirmationEmail(t)
			env.UnsubscribeFromEverything(t.Context(), fixtures.ContactAliceEmail)
		}, "opted out of everything"},
		"suppressed": {func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			env.SetUpAcmeConfirmationEmail(t)
			env.SuppressDestination(t.Context(), fixtures.ContactAliceEmail)
		}, "suppressed"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			tc.before(t, env)

			_, err := askConfirmation(t, requester(t, env), fixtures.ContactAliceID)
			p := testhelper.ExternalRefusal(t, err, http.StatusConflict)
			detail := strings.ToLower(p.Detail.Value)
			queued := env.JobRowsOf(t, confirmationrequests.SendArgs{}.Kind())
			assert.Contains(t, detail, tc.detail)
			assert.Empty(t, queued)
		})
	}
}

// At most three confirmation emails reach an address in 24 hours: a fourth request is a 429 with
// Retry-After and queues nothing.
func TestAFourthConfirmationRequestInADayIsRateLimited(t *testing.T) {
	env := testhelper.Setup(t)
	env.SetUpAcmeConfirmationEmail(t)
	c := requester(t, env)
	for range 3 {
		_, err := askConfirmation(t, c, fixtures.ContactAliceID)
		require.NoError(t, err)
	}

	_, err := askConfirmation(t, c, fixtures.ContactAliceID)
	p := testhelper.ExternalRefusal(t, err, http.StatusTooManyRequests)
	queued := env.JobRowsOf(t, confirmationrequests.SendArgs{}.Kind())
	assert.Positive(t, p.RetryAfter.Value)
	assert.Len(t, queued, 3)
}

// A confirmation request is send-class: a token without confirmations:send is refused.
func TestAConfirmationRequestNeedsTheConfirmationsSendScope(t *testing.T) {
	env := testhelper.Setup(t)
	env.SetUpAcmeConfirmationEmail(t)

	_, err := askConfirmation(t, env.ExternalScoped(t, apitokens.ScopeContactsWrite), fixtures.ContactAliceID)
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}
