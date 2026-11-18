package site_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/event"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// verifiedFrom is an address on Acme's verified Sending domain (SendingDomainVerified).
const verifiedFrom = "hello@mail.acme.com"

func acmeRevision(t *testing.T, env *testhelper.TestEnv) siteapi.Revision {
	t.Helper()
	return siteapi.Revision(env.WorkspaceRevision(t.Context(), t, string(fixtures.AcmePublicID)))
}

func setUpConfirmationEmail(t *testing.T, env *testhelper.TestEnv, c *siteapi.Client, in siteapi.SiteUpdateConfirmationEmailInput) (*siteapi.SiteConfirmationEmailResource, error) {
	t.Helper()
	in.Revision = acmeRevision(t, env)
	return c.SiteWorkspacesUpdateConfirmationEmail(t.Context(), &in,
		siteapi.SiteWorkspacesUpdateConfirmationEmailParams{WorkspaceId: fixtures.AcmePublicID})
}

func setConfirmedOptIn(t *testing.T, env *testhelper.TestEnv, c *siteapi.Client, enabled bool) (*siteapi.SiteWorkspaceResource, error) {
	t.Helper()
	return c.SiteWorkspacesSetConfirmedOptIn(t.Context(),
		&siteapi.SiteSetConfirmedOptInInput{Revision: acmeRevision(t, env), Enabled: enabled},
		siteapi.SiteWorkspacesSetConfirmedOptInParams{WorkspaceId: fixtures.AcmePublicID})
}

// acmeEmailAudience is the Contacts an email Broadcast of Acme reaches now.
func acmeEmailAudience(t *testing.T, env *testhelper.TestEnv) []int64 {
	t.Helper()
	s := env.DB.Scoped(fixtures.AcmeID)
	audience, err := eligibility.New().BroadcastAudience(t.Context(), s, channels.Email, nil)
	require.NoError(t, err)
	ids, err := s.Contact().Query().Where(audience).IDs(t.Context())
	require.NoError(t, err)
	return ids
}

func TestTheConfirmationEmailReadsAsTheDefaultsNamingTheWorkspaceUntilSetUp(t *testing.T) {
	env := testhelper.Setup(t)

	got, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWorkspacesGetConfirmationEmail(t.Context(),
		siteapi.SiteWorkspacesGetConfirmationEmailParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	assert.True(t, got.FromEmail.Null, "not set up")
	assert.True(t, got.FromName.Null)
	assert.Equal(t, fixtures.AcmeName, got.Defaults.FromName)
	assert.Contains(t, got.Defaults.Subject, fixtures.AcmeName)
	assert.Contains(t, got.Defaults.Intro, fixtures.AcmeName)
}

func TestAnAdminSetsUpTheConfirmationEmailAndABlankTextGoesBackToItsDefault(t *testing.T) {
	env := testhelper.Setup(t)
	admin := env.SiteActor(t, fixtures.AdminAliceEmail)

	saved, err := setUpConfirmationEmail(t, env, admin, siteapi.SiteUpdateConfirmationEmailInput{
		FromEmail: verifiedFrom, FromName: siteapi.NewOptString("  Acme news "),
		Subject: siteapi.NewOptString("Please confirm"), Intro: siteapi.NewOptString(" "),
	})
	require.NoError(t, err)

	assert.Equal(t, siteapi.EmailAddress(verifiedFrom), saved.FromEmail.Value)
	assert.Equal(t, "Acme news", saved.FromName.Value)
	assert.Equal(t, "Please confirm", saved.Subject.Value)
	assert.True(t, saved.Intro.Null, "a blank intro is the default")
	entries := testhelper.Queued[*events.AuditEntry](t, env, events.NameAuditEntry)
	require.Len(t, entries, 1)
	assert.Equal(t, events.ActionWorkspaceUpdate, entries[0].Action)
}

func TestAConfirmationFromOutsideAVerifiedSendingDomainIsRefusedOnSave(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	for _, from := range []string{"hello@news.acme.com", "hello@elsewhere.example"} {
		_, err := setUpConfirmationEmail(t, env, owner, siteapi.SiteUpdateConfirmationEmailInput{FromEmail: siteapi.EmailAddress(from)})

		refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
		assert.Contains(t, refused.Errors.Value, "fromEmail", from)
	}
	got, err := owner.SiteWorkspacesGetConfirmationEmail(t.Context(),
		siteapi.SiteWorkspacesGetConfirmationEmailParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.True(t, got.FromEmail.Null)
}

// grandfatheredOfAcme counts Acme's grandfathered Confirmations.
func grandfatheredOfAcme(t *testing.T, env *testhelper.TestEnv) int {
	t.Helper()
	n, err := env.DB.Confirmation.Query().
		Where(confirmation.WorkspaceID(fixtures.AcmeID), confirmation.ProvenanceEQ(confirmation.ProvenanceGrandfathered)).
		Count(t.Context())
	require.NoError(t, err)
	return n
}

func TestTurningConfirmedOptInOnWithoutTheConfirmationEmailIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	before := grandfatheredOfAcme(t, env)

	_, err := setConfirmedOptIn(t, env, env.SiteActor(t, fixtures.OwnerJohnEmail), true)

	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	after := grandfatheredOfAcme(t, env)
	assert.Equal(t, siteapi.ProblemCodeConfirmationEmailRequired, refused.Code.Value)
	assert.Equal(t, before, after, "nothing is grandfathered")
	assert.Empty(t, env.QueuedEvents(t, events.NameAuditEntry))
}

func TestAMemberCannotTurnConfirmedOptInOnNorSetUpItsEmail(t *testing.T) {
	env := testhelper.Setup(t)
	member := env.SiteActor(t, fixtures.MemberMaryEmail)

	_, switched := setConfirmedOptIn(t, env, member, true)
	_, saved := setUpConfirmationEmail(t, env, member, siteapi.SiteUpdateConfirmationEmailInput{FromEmail: verifiedFrom})

	testhelper.SiteRefusal(t, switched, http.StatusForbidden)
	testhelper.SiteRefusal(t, saved, http.StatusForbidden)
}

// Turning the policy on grandfathers the list mailable then (ADR 0013): every Contact a Broadcast
// reached before still receives it, its Confirmation recorded as history that starts no Automation
// and reaches no webhook, and the switch is one Audit entry carrying their number.
func TestTurningConfirmedOptInOnGrandfathersTheMailableListAsHistory(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	_, err := setUpConfirmationEmail(t, env, owner, siteapi.SiteUpdateConfirmationEmailInput{FromEmail: verifiedFrom})
	require.NoError(t, err)
	before := acmeEmailAudience(t, env)
	require.NotEmpty(t, before)
	historyOf := func() int {
		n, err := env.DB.Event.Query().
			Where(event.WorkspaceID(fixtures.AcmeID), event.Action(systemevent.MarketingConfirmed), event.Backfill(true)).
			Count(t.Context())
		require.NoError(t, err)
		return n
	}
	grandfatheredBefore, historyBefore := grandfatheredOfAcme(t, env), historyOf()

	ws, err := setConfirmedOptIn(t, env, owner, true)
	require.NoError(t, err)

	after := acmeEmailAudience(t, env)
	grandfathered := grandfatheredOfAcme(t, env) - grandfatheredBefore
	history := historyOf() - historyBefore
	assert.True(t, ws.RequireConfirmedOptIn)
	assert.ElementsMatch(t, before, after, "the next Broadcast reaches the same audience")
	require.Positive(t, grandfathered)
	assert.Equal(t, grandfathered, history, "each Confirmation has its Event, as history")

	for _, args := range env.JobsOf(t, jobs.EvaluateTriggerArgs{}.Kind()) {
		assert.NotEqual(t, systemevent.MarketingConfirmed, args["action"], "no Automation starts")
	}
	for _, queued := range env.EventJobs(t, jobs.WebhookEventArgs{}.Kind()) {
		assert.NotEqual(t, systemevent.MarketingConfirmed, queued.Name, "no webhook endpoint receives it")
	}
	env.PerformEnqueuedJobs(t, jobs.EvaluateTriggerArgs{})
	enrolled, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationOnMarketingConfirmedID)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, enrolled, "the Automation triggered by marketing.confirmed enrolls nobody")

	entries := testhelper.Queued[*events.AuditEntry](t, env, events.NameAuditEntry)
	require.Len(t, entries, 2, "the email's set-up and the switch")
	diff, err := json.Marshal(entries[1].Diff)
	require.NoError(t, err)
	assert.JSONEq(t, `{"require_confirmed_opt_in":{"from":false,"to":true},"confirmations_recorded":`+strconv.Itoa(grandfathered)+`}`, string(diff))
}

func TestAContactCreatedAfterConfirmedOptInIsUnconfirmed(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	_, err := setUpConfirmationEmail(t, env, owner, siteapi.SiteUpdateConfirmationEmailInput{FromEmail: verifiedFrom})
	require.NoError(t, err)
	_, err = setConfirmedOptIn(t, env, owner, true)
	require.NoError(t, err)

	created, err := owner.SiteContactsCreate(t.Context(), &siteapi.SiteCreateContactInput{
		Email: siteapi.NewOptNilEmailAddress("late.comer@example.org"),
	}, siteapi.SiteContactsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	decision, err := eligibility.New().Check(t.Context(), env.DB.Scoped(fixtures.AcmeID),
		eligibility.CheckInput{Channel: channels.Email, Destination: "late.comer@example.org", Source: eligibility.SourceBroadcasts})
	require.NoError(t, err)
	audience := acmeEmailAudience(t, env)
	assert.Equal(t, skipreason.Unconfirmed, decision.Reason)
	assert.NotContains(t, audience, int64(created.ID))
}

// Turning it on while it is on grandfathers nobody who came since; turning it off keeps every
// Confirmation, and is one Audit entry with none recorded.
func TestASwitchToTheStateTheWorkspaceIsInChangesNothingAndOffKeepsTheConfirmations(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	_, err := setUpConfirmationEmail(t, env, owner, siteapi.SiteUpdateConfirmationEmailInput{FromEmail: verifiedFrom})
	require.NoError(t, err)
	_, err = setConfirmedOptIn(t, env, owner, true)
	require.NoError(t, err)
	count := func() int {
		n, err := env.DB.Confirmation.Query().Where(confirmation.WorkspaceID(fixtures.AcmeID)).Count(t.Context())
		require.NoError(t, err)
		return n
	}
	confirmed := count()
	_, err = owner.SiteContactsCreate(t.Context(), &siteapi.SiteCreateContactInput{
		Email: siteapi.NewOptNilEmailAddress("late.comer@example.org"),
	}, siteapi.SiteContactsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	again, err := setConfirmedOptIn(t, env, owner, true)
	require.NoError(t, err)
	afterAgain := count()
	off, err := setConfirmedOptIn(t, env, owner, false)
	require.NoError(t, err)
	afterOff := count()

	assert.True(t, again.RequireConfirmedOptIn)
	assert.Equal(t, confirmed, afterAgain, "nobody who came since is grandfathered")
	assert.False(t, off.RequireConfirmedOptIn)
	assert.Equal(t, confirmed, afterOff, "off keeps every Confirmation")

	entries := lo.Filter(testhelper.Queued[*events.AuditEntry](t, env, events.NameAuditEntry),
		func(e *events.AuditEntry, _ int) bool { return e.Action == events.ActionWorkspaceUpdate })
	require.Len(t, entries, 3, "the set-up, on and off: the repeat is not recorded")
	diff, err := json.Marshal(entries[2].Diff)
	require.NoError(t, err)
	assert.JSONEq(t, `{"require_confirmed_opt_in":{"from":true,"to":false},"confirmations_recorded":0}`, string(diff))
}

func TestTurningConfirmedOptInOnNamesTheRevisionItWasMadeOn(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	_, err := setUpConfirmationEmail(t, env, owner, siteapi.SiteUpdateConfirmationEmailInput{FromEmail: verifiedFrom})
	require.NoError(t, err)

	_, err = owner.SiteWorkspacesSetConfirmedOptIn(context.Background(),
		&siteapi.SiteSetConfirmedOptInInput{Revision: acmeRevision(t, env) - 1, Enabled: true},
		siteapi.SiteWorkspacesSetConfirmedOptInParams{WorkspaceId: fixtures.AcmePublicID})

	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Equal(t, siteapi.ProblemCodeRevisionChanged, refused.Code.Value)
}
