package site_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/sending/confirmationrequests"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// confirmLinkToken is the token of the confirmation link in a message.
var confirmLinkToken = regexp.MustCompile(`/e/confirm/([A-Za-z0-9_\-.]+)`)

func confirmationMailTo(env *testhelper.TestEnv, address string) []messaging.EmailMessage {
	return lo.Filter(env.CustomerMail.Messages(), func(m messaging.EmailMessage, _ int) bool { return m.To == address })
}

// A Member sends a Contact the confirmation email from its page in the console (ADR 0013).
func TestAMemberSendsAConfirmationRequestFromTheConsole(t *testing.T) {
	env := testhelper.Setup(t)
	env.SetUpAcmeConfirmationEmail(t)

	res, err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteConfirmationRequestsCreate(t.Context(),
		siteapi.SiteConfirmationRequestsCreateParams{WorkspaceId: fixtures.AcmePublicID, ContactId: siteapi.EntityId(fixtures.ContactAliceID)})
	require.NoError(t, err)
	assert.Equal(t, &siteapi.SiteConfirmationRequestsCreateAccepted{Status: siteapi.ConfirmationRequestStatusAccepted}, res)
	env.PerformEnqueuedJobs(t, confirmationrequests.SendArgs{})

	mail := confirmationMailTo(env, fixtures.ContactAliceEmail)
	assert.Len(t, mail, 1)
}

// The page of an expired link offers a new one, a request the person makes: an opt-out of
// everything does not stop it, and confirming the new link lifts that opt-out and records the
// Confirmation (ADR 0013). The page is in the Workspace's brand, read now.
func TestTheRenewalOfAnExpiredLinkReachesAnAddressThatLeftEverything(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.SetUpAcmeConfirmationEmail(t)
	env.UnsubscribeFromEverything(ctx, fixtures.ContactAliceEmail)
	anon := env.SiteAnonymous(t)
	expired := env.ExpiredConfirmToken(t, linktarget.Confirmation{
		Destination: fixtures.ContactAliceEmail, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID,
	})

	page, err := anon.SitePublicConfirmationsRead(ctx, siteapi.SitePublicConfirmationsReadParams{Token: expired})
	require.NoError(t, err)
	assert.True(t, page.Expired)
	assert.Equal(t, fixtures.AcmeName, page.WorkspaceName)
	assert.Equal(t, "#1a73e8", page.Brand.AccentColor, "Acme's Brand kit")

	renewed, err := anon.SitePublicConfirmationsRenew(ctx, siteapi.SitePublicConfirmationsRenewParams{Token: expired})
	require.NoError(t, err)
	assert.Equal(t, siteapi.ConfirmationRequestStatusAccepted, renewed.Status)
	env.PerformEnqueuedJobs(t, confirmationrequests.SendArgs{})
	mail := confirmationMailTo(env, fixtures.ContactAliceEmail)
	require.Len(t, mail, 1, "the opt-out of everything does not stop the person's request")
	link := confirmLinkToken.FindStringSubmatch(mail[0].HTML)
	require.Len(t, link, 2)

	err = anon.SitePublicConfirmationsPerform(ctx, siteapi.SitePublicConfirmationsPerformParams{Token: link[1]})
	require.NoError(t, err)
	d, err := eligibility.New().Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.CheckInput{
		Channel: channels.Email, Destination: fixtures.ContactAliceEmail, Mode: eligibility.Marketing(eligibility.SourceBroadcasts),
	})
	require.NoError(t, err)
	resubscribed := env.QueuedMessages(t, systemevent.EmailResubscribed)
	confirmed := env.QueuedMessages(t, systemevent.MarketingConfirmed)
	assert.True(t, d.Eligible, "the opt-out is lifted")
	assert.Len(t, resubscribed, 1)
	assert.Len(t, confirmed, 1)
}

// The person's request says nothing about an address: one the link's Contact no longer has, a
// suppressed one, and one over its limit are answered as accepted, and get no email.
func TestTheRenewalSaysNothingAboutTheAddress(t *testing.T) {
	cases := map[string]struct {
		before func(t *testing.T, env *testhelper.TestEnv)
		target linktarget.Confirmation
	}{
		"no longer the contact's": {func(*testing.T, *testhelper.TestEnv) {}, linktarget.Confirmation{
			Destination: "someone.else@example.com", WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID,
		}},
		"suppressed": {func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			env.SuppressDestination(t.Context(), fixtures.ContactAliceEmail)
		}, linktarget.Confirmation{Destination: fixtures.ContactAliceEmail, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID}},
		"over its limit": {func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			for range 3 {
				_, err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteConfirmationRequestsCreate(t.Context(),
					siteapi.SiteConfirmationRequestsCreateParams{WorkspaceId: fixtures.AcmePublicID, ContactId: siteapi.EntityId(fixtures.ContactAliceID)})
				require.NoError(t, err)
			}
		}, linktarget.Confirmation{Destination: fixtures.ContactAliceEmail, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			env.SetUpAcmeConfirmationEmail(t)
			tc.before(t, env)
			before := env.JobRowsOf(t, confirmationrequests.SendArgs{}.Kind())

			token := env.ExpiredConfirmToken(t, tc.target)

			renewed, err := env.SiteAnonymous(t).SitePublicConfirmationsRenew(t.Context(), siteapi.SitePublicConfirmationsRenewParams{Token: token})
			require.NoError(t, err)
			after := env.JobRowsOf(t, confirmationrequests.SendArgs{}.Kind())
			assert.Equal(t, siteapi.ConfirmationRequestStatusAccepted, renewed.Status)
			assert.Len(t, after, len(before), "nothing more is queued")
		})
	}
}

// A token that is no link of ours is a 400 on the page.
func TestTheConfirmationPageOfAnInvalidToken(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteAnonymous(t).SitePublicConfirmationsRead(t.Context(), siteapi.SitePublicConfirmationsReadParams{Token: "garbage"})
	testhelper.SiteRefusal(t, err, http.StatusBadRequest)
}
