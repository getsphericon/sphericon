package outboundmessages_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// confirmLink finds the confirmation link in a rendered message.
var confirmLink = regexp.MustCompile(regexp.QuoteMeta(trackingBase+"/e/confirm/") + `([A-Za-z0-9_\-.]+)`)

// acmeConfirmation is the confirmation email Acme sets up, its texts holding what Liquid and HTML
// would read as their own.
var acmeConfirmation = workspaces.ConfirmationEmail{
	FromEmail: "hello@" + fixtures.SendingDomainVerifiedDomain, FromName: "Acme",
	Subject: "Confirm {{ email }}", Intro: "Hi <b>there</b>",
}

func confirmationTo(t *testing.T, env *testhelper.TestEnv, key string, contactID int64, by outboundmessages.ConfirmationAsker) outboundmessages.Request {
	t.Helper()
	c := env.DB.Contact.GetX(t.Context(), contactID)
	return outboundmessages.Confirmation(outboundmessages.ConfirmationMessage{Request: key, Contact: c, AskedBy: by, Email: acmeConfirmation})
}

// The confirmation email (ADR 0013) is the Workspace's From, subject and intro, as written, in a
// fixed frame: the Confirm button carrying the link, the line that nothing happens, the postal
// address and the Brand kit's accent. It carries nothing of marketing: no unsubscribe link or
// header, no pixel, no rewritten link. The link names the address and its Contact.
func TestConfirmationEmailCarriesItsLinkAndNothingOfMarketing(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)

	res, err := newModule(env, time.Now).Send(t.Context(), s, confirmationTo(t, env, "confirmation:test", fixtures.ContactAliceID, outboundmessages.AskedByWorkspace))
	require.NoError(t, err)
	require.Equal(t, outboundmessages.Sent, res.Outcome)
	msgs := env.CustomerMail.Messages()
	msg := msgs[len(msgs)-1]

	assert.Equal(t, "Confirm {{ email }}", msg.Subject, "the subject is plain text, not a template")
	assert.Equal(t, acmeConfirmation.FromEmail, msg.From)
	assert.Equal(t, fixtures.ContactAliceEmail, msg.To)
	assert.Contains(t, msg.HTML, "Hi &lt;b&gt;there&lt;/b&gt;", "the intro is text")
	assert.Contains(t, msg.HTML, "100 Market Street")
	assert.Contains(t, msg.HTML, "#1a73e8", "the Brand kit's accent")
	assert.Empty(t, msg.ListUnsubscribeURL)
	for _, link := range []string{trackingBase + "/e/o/", trackingBase + "/e/c/", trackingBase + "/e/u/"} {
		assert.NotContains(t, msg.HTML, link, "no pixel, rewritten link or unsubscribe link")
	}
	found := confirmLink.FindStringSubmatch(msg.HTML)
	require.Len(t, found, 2, "the Confirm button carries the link")
	assert.Contains(t, msg.Text, found[0], "the text part carries the link")
	target, err := messagetoken.New("test-secret", trackingBase, time.Now).DecodeConfirm(found[1])
	require.NoError(t, err)
	assert.Equal(t, fixtures.ContactAliceEmail, target.Destination)
	assert.EqualValues(t, fixtures.ContactAliceID, target.ContactID)
	assert.EqualValues(t, fixtures.AcmeID, target.WorkspaceID)

	row, err := s.OutboundMessage().Get(t.Context(), res.MessageID)
	require.NoError(t, err)
	assert.Equal(t, outboundmessage.KindConfirmation, row.Kind)
	assert.Nil(t, row.SendingSource)
}

// Who asked decides what an opt-out of everything does: the Workspace's request is Skipped for it,
// the person's goes out.
func TestConfirmationEmailIsCheckedInTheModeOfWhoAsked(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := newModule(env, time.Now)
	env.UnsubscribeFromEverything(t.Context(), fixtures.ContactAliceEmail)

	byWorkspace, err := m.Send(t.Context(), s, confirmationTo(t, env, "confirmation:ws", fixtures.ContactAliceID, outboundmessages.AskedByWorkspace))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Skipped, byWorkspace.Outcome)
	assert.Equal(t, skipreason.UnsubscribedEverything, byWorkspace.SkippedReason)

	byPerson, err := m.Send(t.Context(), s, confirmationTo(t, env, "confirmation:person", fixtures.ContactAliceID, outboundmessages.AskedByPerson))
	require.NoError(t, err)
	assert.Equal(t, outboundmessages.Sent, byPerson.Outcome)
}
