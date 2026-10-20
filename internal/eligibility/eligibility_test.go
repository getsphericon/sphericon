package eligibility_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suppression"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// enableConfirmedOptIn turns the workspace's require-confirmed-opt-in policy on
// (ADR 0013). txdb rolls it back at test end.
func enableConfirmedOptIn(t *testing.T, db *ent.Client, ctx context.Context) {
	t.Helper()
	_, err := db.Workspace.UpdateOneID(fixtures.AcmeID).SetRequireConfirmedOptIn(true).Save(ctx)
	require.NoError(t, err)
}

func TestCheckEligibleByDefault(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	d, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail,
		"fresh@example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.True(t, d.Eligible)
	assert.Empty(t, d.Reason)
}

func TestCheckSuppressed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := env.DB.Suppression.Create().SetWorkspaceID(fixtures.AcmeID).
		SetChannel(entsuppression.ChannelEmail).SetDestination("blocked@example.com").
		SetReason(entsuppression.ReasonComplaint).Save(ctx)
	require.NoError(t, err)

	d, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail,
		"blocked@example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.False(t, d.Eligible)
	assert.Equal(t, eligibility.ReasonSuppressed, d.Reason)
}

// Transactional sends (source "") skip layers 2–3 but still
// respect Suppression.
func TestCheckTransactionalSkipsUnsubscribeButNotSuppression(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// Unsubscribed from everything — yet a transactional send must still go.
	_, err := env.DB.Unsubscribe.Create().SetWorkspaceID(fixtures.AcmeID).
		SetChannel(unsubscribe.ChannelEmail).SetDestination("txn@example.com").
		SetSendingSource(eligibility.SourceEverything).Save(ctx)
	require.NoError(t, err)

	d, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail,
		"txn@example.com", "")
	require.NoError(t, err)
	assert.True(t, d.Eligible, "transactional skips unsubscribe layers")

	// But a suppressed destination is never sent to, transactional or not.
	_, err = env.DB.Suppression.Create().SetWorkspaceID(fixtures.AcmeID).
		SetChannel(entsuppression.ChannelEmail).SetDestination("txn@example.com").
		SetReason(entsuppression.ReasonBounce).Save(ctx)
	require.NoError(t, err)
	d, err = eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail,
		"txn@example.com", "")
	require.NoError(t, err)
	assert.False(t, d.Eligible)
	assert.Equal(t, eligibility.ReasonSuppressed, d.Reason)
}

func TestCheckUnsubscribedEverythingVsSource(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := env.DB.Unsubscribe.Create().SetWorkspaceID(fixtures.AcmeID).
		SetChannel(unsubscribe.ChannelEmail).SetDestination("evt@example.com").
		SetSendingSource(eligibility.SourceEverything).Save(ctx)
	require.NoError(t, err)
	d, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail,
		"evt@example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, eligibility.ReasonUnsubscribedEverything, d.Reason)

	src := eligibility.AutomationSource(42)
	_, err = env.DB.Unsubscribe.Create().SetWorkspaceID(fixtures.AcmeID).
		SetChannel(unsubscribe.ChannelEmail).SetDestination("src@example.com").
		SetSendingSource(src).Save(ctx)
	require.NoError(t, err)
	// Ineligible for that source...
	d, err = eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail, "src@example.com", src)
	require.NoError(t, err)
	assert.Equal(t, eligibility.ReasonUnsubscribedSource, d.Reason)
	// ...but eligible for a different source.
	d, err = eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail,
		"src@example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.True(t, d.Eligible)
}

// The confirmation gate (ADR 0013) is a no-op when the workspace does not require it: an
// address with no Confirmation is still eligible, and the batch predicate still
// matches it. This is the "confirmed-opt-in off ⇒ behavior unchanged" invariant.
func TestConfirmationGateNoopWhenOff(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	d, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail,
		"unconfirmed@example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.True(t, d.Eligible, "no confirmation required when the gate is off")

	c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("unconf@example.com").Save(ctx)
	require.NoError(t, err)
	matched, err := env.DB.Contact.Query().
		Where(contact.ID(c.ID),
			eligibility.Predicate(eligibility.ChannelEmail, eligibility.SourceBroadcasts)).
		Exist(ctx)
	require.NoError(t, err)
	assert.True(t, matched, "unconfirmed contact still in the audience when the gate is off")
}

// When the workspace requires confirmed opt-in, an unconfirmed address is blocked and confirming
// it makes it mailable — via both the point Check and the batch Predicate.
func TestConfirmationGateWhenOn(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	enableConfirmedOptIn(t, env.DB, ctx)

	c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("gate@example.com").Save(ctx)
	require.NoError(t, err)

	inAudience := func() bool {
		ok, err := env.DB.Contact.Query().
			Where(contact.ID(c.ID),
				eligibility.Predicate(eligibility.ChannelEmail, eligibility.SourceBroadcasts)).
			Exist(ctx)
		require.NoError(t, err)
		return ok
	}

	// Unconfirmed: blocked.
	d, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail,
		"gate@example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.False(t, d.Eligible)
	assert.Equal(t, eligibility.ReasonUnconfirmed, d.Reason)
	assert.False(t, inAudience(), "unconfirmed contact excluded from the audience")

	// Confirm it: now mailable.
	_, err = env.DB.Confirmation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetChannel(confirmation.ChannelEmail).SetDestination("gate@example.com").
		SetProvenance(confirmation.ProvenanceDoubleOptIn).Save(ctx)
	require.NoError(t, err)

	d, err = eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail,
		"gate@example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.True(t, d.Eligible)
	assert.True(t, inAudience(), "confirmed contact included in the audience")
}

// Confirmation is the lowest-priority layer: a negative always dominates. A
// confirmed-but-suppressed address is still ineligible (and reports the negative).
func TestConfirmationDoesNotOverrideNegatives(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	enableConfirmedOptIn(t, env.DB, ctx)

	_, err := env.DB.Confirmation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetChannel(confirmation.ChannelEmail).SetDestination("both@example.com").
		SetProvenance(confirmation.ProvenanceDoubleOptIn).Save(ctx)
	require.NoError(t, err)
	_, err = env.DB.Suppression.Create().SetWorkspaceID(fixtures.AcmeID).
		SetChannel(entsuppression.ChannelEmail).SetDestination("both@example.com").
		SetReason(entsuppression.ReasonComplaint).Save(ctx)
	require.NoError(t, err)

	d, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail,
		"both@example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.False(t, d.Eligible)
	assert.Equal(t, eligibility.ReasonSuppressed, d.Reason, "negatives dominate the confirmation")
}

// The batch Predicate folds case in SQL: a mixed-case contact email matches a
// lower-cased suppression destination and is excluded from the query.
func TestPredicateFoldsCaseAndExcludes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("Caps@Example.com").Save(ctx)
	require.NoError(t, err)
	_, err = env.DB.Suppression.Create().SetWorkspaceID(fixtures.AcmeID).
		SetChannel(entsuppression.ChannelEmail).SetDestination("caps@example.com").
		SetReason(entsuppression.ReasonBounce).Save(ctx)
	require.NoError(t, err)

	matched, err := env.DB.Contact.Query().
		Where(contact.ID(c.ID),
			eligibility.Predicate(eligibility.ChannelEmail, eligibility.SourceBroadcasts)).
		Exist(ctx)
	require.NoError(t, err)
	assert.False(t, matched, "suppressed contact excluded despite case difference")
}

// Stored destinations are lower-cased; a mixed-case contact email must still match.
func TestCheckFoldsCase(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := env.DB.Suppression.Create().SetWorkspaceID(fixtures.AcmeID).
		SetChannel(entsuppression.ChannelEmail).SetDestination("mixed@example.com").
		SetReason(entsuppression.ReasonBounce).Save(ctx)
	require.NoError(t, err)

	d, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail,
		"  Mixed@Example.com  ", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.False(t, d.Eligible, "input is normalized before lookup")
}

// The batch Predicate and the point Check are two shapes of one rule. For every
// combination of opt-out state, source and policy they must agree — this is the
// parity guarantee that replaced two hand-written copies of the layers.
func TestPredicateAndCheckAgree(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	auto := eligibility.AutomationSource(7)
	type fixture struct {
		name      string
		email     string
		suppress  bool
		unsub     string // sending source, "" = none
		confirmed bool
	}
	cases := []fixture{
		{name: "clean", email: "p-clean@example.com"},
		{name: "suppressed", email: "p-sup@example.com", suppress: true},
		{name: "unsub everything", email: "p-every@example.com", unsub: eligibility.SourceEverything},
		{name: "unsub broadcasts", email: "p-bcast@example.com", unsub: eligibility.SourceBroadcasts},
		{name: "unsub automation", email: "p-auto@example.com", unsub: auto},
		{name: "confirmed", email: "p-conf@example.com", confirmed: true},
		{name: "confirmed and suppressed", email: "p-both@example.com", suppress: true, confirmed: true},
		{name: "mixed case", email: "P-Case@Example.com", suppress: true},
	}
	ids := map[string]int64{}
	for _, f := range cases {
		c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail(f.email).Save(ctx)
		require.NoError(t, err)
		ids[f.name] = c.ID
	}
	for _, f := range lo.Filter(cases, func(f fixture, _ int) bool { return f.suppress }) {
		_, err := env.DB.Suppression.Create().SetWorkspaceID(fixtures.AcmeID).
			SetChannel(entsuppression.ChannelEmail).SetDestination(suppression.NormalizeDestination(f.email)).
			SetReason(entsuppression.ReasonBounce).Save(ctx)
		require.NoError(t, err)
	}
	for _, f := range lo.Filter(cases, func(f fixture, _ int) bool { return f.unsub != "" }) {
		_, err := env.DB.Unsubscribe.Create().SetWorkspaceID(fixtures.AcmeID).
			SetChannel(unsubscribe.ChannelEmail).SetDestination(suppression.NormalizeDestination(f.email)).
			SetSendingSource(f.unsub).Save(ctx)
		require.NoError(t, err)
	}
	for _, f := range lo.Filter(cases, func(f fixture, _ int) bool { return f.confirmed }) {
		_, err := env.DB.Confirmation.Create().SetWorkspaceID(fixtures.AcmeID).
			SetChannel(confirmation.ChannelEmail).SetDestination(suppression.NormalizeDestination(f.email)).
			SetProvenance(confirmation.ProvenanceDoubleOptIn).Save(ctx)
		require.NoError(t, err)
	}

	for _, requireConfirmed := range []bool{false, true} {
		_, err := env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetRequireConfirmedOptIn(requireConfirmed).Save(ctx)
		require.NoError(t, err)
		for _, source := range []string{"", eligibility.SourceBroadcasts, auto} {
			for _, f := range cases {
				d, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail, f.email, source)
				require.NoError(t, err)
				inAudience, err := env.DB.Contact.Query().
					Where(contact.ID(ids[f.name]), eligibility.Predicate(eligibility.ChannelEmail, source)).
					Exist(ctx)
				require.NoError(t, err)
				assert.Equal(t, d.Eligible, inAudience,
					"%s (source %q, requireConfirmed %v): Check and Predicate disagree", f.name, source, requireConfirmed)
			}
		}
	}
}

// A Transactional destination may have no Contact at all; Suppression still binds.
func TestCheckWorksWithoutAContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := env.DB.Suppression.Create().SetWorkspaceID(fixtures.AcmeID).
		SetChannel(entsuppression.ChannelEmail).SetDestination("nocontact@example.com").
		SetReason(entsuppression.ReasonComplaint).Save(ctx)
	require.NoError(t, err)
	d, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail, "nocontact@example.com", "")
	require.NoError(t, err)
	assert.Equal(t, eligibility.ReasonSuppressed, d.Reason)
}
