package eligibility_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The scenarios live in two fixture Workspaces: Tyrell keeps the default policy (confirmed
// opt-in off), Lumon requires it (ADR 0013). Both hold the same matrix of Contacts, one per
// opt-out state (fixtures/contacts.yml, "Eligibility matrix").

// inAudience reports whether the batch predicate keeps the Contact.
func inAudience(t *testing.T, env *testhelper.TestEnv, ws, contactID int64, source string) bool {
	t.Helper()
	ok, err := env.DB.Scoped(ws).Contact().Query().
		Where(contact.ID(contactID), eligibility.Predicate(eligibility.ChannelEmail, source)).
		Exist(context.Background())
	require.NoError(t, err)
	return ok
}

// decide runs the point check for a destination in a Workspace.
func decide(t *testing.T, env *testhelper.TestEnv, ws int64, destination, source string) eligibility.Decision {
	t.Helper()
	d, err := eligibility.Check(context.Background(), env.DB.Scoped(ws), eligibility.ChannelEmail, destination, source)
	require.NoError(t, err)
	return d
}

func TestCheckEligibleByDefault(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.AcmeID, "fresh@example.com", eligibility.SourceBroadcasts)
	assert.True(t, d.Eligible)
	assert.Empty(t, d.Reason)
}

func TestCheckSuppressed(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.TyrellID, fixtures.SuppressionTyrellSuppressedDestination, eligibility.SourceBroadcasts)
	assert.False(t, d.Eligible)
	assert.Equal(t, eligibility.ReasonSuppressed, d.Reason)
}

// A lifted Suppression no longer blocks, in a single check or in the contact predicate.
func TestLiftedSuppressionDoesNotBlock(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.AcmeID, "lifted@codebasics.dev", eligibility.SourceBroadcasts)
	assert.True(t, d.Eligible, "the fixture's lifted destination is mailable")
	assert.True(t, inAudience(t, env, fixtures.AcmeID, fixtures.ContactLiftedID, eligibility.SourceBroadcasts),
		"an audience keeps the contact of a lifted destination")
}

// Transactional sends (source "") skip the unsubscribe and confirmation layers.
func TestCheckTransactionalSkipsUnsubscribe(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.TyrellID, fixtures.TyrellMatrixUnsubEverythingEmail, "")
	assert.True(t, d.Eligible, "transactional skips unsubscribe layers")
}

// A suppressed destination is never sent to, transactional or not, even when it also opted out.
func TestCheckTransactionalRespectsSuppression(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.TyrellID, fixtures.SuppressionTyrellTransactionalDestination, "")
	assert.False(t, d.Eligible)
	assert.Equal(t, eligibility.ReasonSuppressed, d.Reason)
}

func TestCheckUnsubscribedEverything(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.TyrellID, fixtures.TyrellMatrixUnsubEverythingEmail, eligibility.SourceBroadcasts)
	assert.Equal(t, eligibility.ReasonUnsubscribedEverything, d.Reason)
}

func TestCheckUnsubscribedFromOneSourceOnly(t *testing.T) {
	env := testhelper.Setup(t)
	src := eligibility.AutomationSource(7)

	assert.Equal(t, eligibility.ReasonUnsubscribedSource,
		decide(t, env, fixtures.TyrellID, fixtures.TyrellMatrixUnsubAutomationEmail, src).Reason)
	assert.True(t, decide(t, env, fixtures.TyrellID, fixtures.TyrellMatrixUnsubAutomationEmail, eligibility.SourceBroadcasts).Eligible,
		"eligible for a different source")
}

// The confirmation gate (ADR 0013) is a no-op when the workspace does not require it: an
// address with no Confirmation is still eligible, and the batch predicate still
// matches it. This is the "confirmed-opt-in off ⇒ behavior unchanged" invariant.
func TestConfirmationGateNoopWhenOff(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.TyrellID, fixtures.TyrellMatrixCleanEmail, eligibility.SourceBroadcasts)
	assert.True(t, d.Eligible, "no confirmation required when the gate is off")
	assert.True(t, inAudience(t, env, fixtures.TyrellID, fixtures.TyrellMatrixCleanID, eligibility.SourceBroadcasts),
		"unconfirmed contact still in the audience when the gate is off")
}

// When the workspace requires confirmed opt-in, an unconfirmed address is blocked, via both
// the point Check and the batch Predicate.
func TestConfirmationGateBlocksUnconfirmed(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.LumonID, fixtures.LumonMatrixCleanEmail, eligibility.SourceBroadcasts)
	assert.False(t, d.Eligible)
	assert.Equal(t, eligibility.ReasonUnconfirmed, d.Reason)
	assert.False(t, inAudience(t, env, fixtures.LumonID, fixtures.LumonMatrixCleanID, eligibility.SourceBroadcasts),
		"unconfirmed contact excluded from the audience")
}

// A confirmed address is mailable under the gate, via both the point Check and the Predicate.
func TestConfirmationGateAdmitsConfirmed(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.LumonID, fixtures.LumonMatrixConfirmedEmail, eligibility.SourceBroadcasts)
	assert.True(t, d.Eligible)
	assert.True(t, inAudience(t, env, fixtures.LumonID, fixtures.LumonMatrixConfirmedID, eligibility.SourceBroadcasts),
		"confirmed contact included in the audience")
}

// Confirmation is the lowest-priority layer: a negative always dominates. A
// confirmed-but-suppressed address is still ineligible (and reports the negative).
func TestConfirmationDoesNotOverrideNegatives(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.LumonID, fixtures.LumonMatrixConfirmedSuppressedEmail, eligibility.SourceBroadcasts)
	assert.False(t, d.Eligible)
	assert.Equal(t, eligibility.ReasonSuppressed, d.Reason, "negatives dominate the confirmation")
}

// The batch Predicate folds case in SQL: a mixed-case contact email matches a
// lower-cased suppression destination and is excluded from the query.
func TestPredicateFoldsCaseAndExcludes(t *testing.T) {
	env := testhelper.Setup(t)

	assert.False(t, inAudience(t, env, fixtures.TyrellID, fixtures.TyrellMatrixMixedCaseID, eligibility.SourceBroadcasts),
		"suppressed contact excluded despite case difference")
}

// Stored destinations are lower-cased; a mixed-case address must still match.
func TestCheckFoldsCase(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.TyrellID, "  "+fixtures.TyrellMatrixMixedCaseEmail+"  ", eligibility.SourceBroadcasts)
	assert.False(t, d.Eligible, "input is normalized before lookup")
}

// A destination may have no Contact at all; Suppression still binds.
func TestCheckWorksWithoutAContact(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.TyrellID, fixtures.SuppressionTyrellNoContactDestination, "")
	assert.Equal(t, eligibility.ReasonSuppressed, d.Reason)
}

// matrixEligibility returns the emails of a Workspace's matrix Contacts that the point Check
// admits and that the batch Predicate keeps, for one source.
func matrixEligibility(t *testing.T, env *testhelper.TestEnv, ws int64, source string) (byCheck, byPredicate []string) {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(ws)
	contacts, err := s.Contact().Query().All(ctx)
	require.NoError(t, err)
	for _, c := range contacts {
		d, err := eligibility.Check(ctx, s, eligibility.ChannelEmail, *c.Email, source)
		require.NoError(t, err)
		if d.Eligible {
			byCheck = append(byCheck, *c.Email)
		}
	}
	kept, err := s.Contact().Query().Where(eligibility.Predicate(eligibility.ChannelEmail, source)).All(ctx)
	require.NoError(t, err)
	for _, c := range kept {
		byPredicate = append(byPredicate, *c.Email)
	}
	return byCheck, byPredicate
}

// The batch Predicate and the point Check are two shapes of one rule. For every opt-out state
// and source they must agree, under both policies: this is the parity guarantee that replaced
// two hand-written copies of the layers. The expected audiences are stated, so a rule that
// drifts in both shapes at once fails too.
func TestPredicateAndCheckAgree(t *testing.T) {
	env := testhelper.Setup(t)

	for name, tc := range map[string]struct {
		ws     int64
		source string
		want   []string
	}{
		"gate off, transactional": {fixtures.TyrellID, "", []string{
			fixtures.TyrellMatrixCleanEmail, fixtures.TyrellMatrixUnsubEverythingEmail, fixtures.TyrellMatrixUnsubBroadcastsEmail,
			fixtures.TyrellMatrixUnsubAutomationEmail, fixtures.TyrellMatrixConfirmedEmail,
		}},
		"gate off, broadcasts": {fixtures.TyrellID, eligibility.SourceBroadcasts, []string{
			fixtures.TyrellMatrixCleanEmail, fixtures.TyrellMatrixUnsubAutomationEmail, fixtures.TyrellMatrixConfirmedEmail,
		}},
		"gate off, automation": {fixtures.TyrellID, eligibility.AutomationSource(7), []string{
			fixtures.TyrellMatrixCleanEmail, fixtures.TyrellMatrixUnsubBroadcastsEmail, fixtures.TyrellMatrixConfirmedEmail,
		}},
		"gate on, transactional": {fixtures.LumonID, "", []string{
			fixtures.LumonMatrixCleanEmail, fixtures.LumonMatrixUnsubEverythingEmail, fixtures.LumonMatrixUnsubBroadcastsEmail,
			fixtures.LumonMatrixUnsubAutomationEmail, fixtures.LumonMatrixConfirmedEmail,
		}},
		"gate on, broadcasts": {fixtures.LumonID, eligibility.SourceBroadcasts, []string{fixtures.LumonMatrixConfirmedEmail}},
		"gate on, automation": {fixtures.LumonID, eligibility.AutomationSource(7), []string{fixtures.LumonMatrixConfirmedEmail}},
	} {
		byCheck, byPredicate := matrixEligibility(t, env, tc.ws, tc.source)
		assert.ElementsMatch(t, tc.want, byCheck, "%s: point Check", name)
		assert.ElementsMatch(t, tc.want, byPredicate, "%s: batch Predicate", name)
	}
}
