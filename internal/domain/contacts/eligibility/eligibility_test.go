package eligibility_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The scenarios live in two fixture Workspaces: Tyrell keeps the default policy (confirmed
// opt-in off), Lumon requires it (ADR 0013). Both hold the same matrix of Contacts, one per
// opt-out state (fixtures/contacts.yml, "Eligibility matrix").

// inAudience reports whether the batch predicate keeps the Contact.
func inAudience(t *testing.T, env *testhelper.TestEnv, ws, contactID int64, mode eligibility.Mode) bool {
	t.Helper()
	ok, err := env.DB.Scoped(ws).Contact().Query().
		Where(contact.ID(contactID), eligibility.Audience(eligibility.EmailDestination(), mode)).
		Exist(context.Background())
	require.NoError(t, err)
	return ok
}

// decide runs the point check for a destination in a Workspace.
func decide(t *testing.T, env *testhelper.TestEnv, ws int64, destination string, mode eligibility.Mode) eligibility.Decision {
	t.Helper()
	d, err := eligibility.New().Check(context.Background(), env.DB.Scoped(ws), eligibility.CheckInput{Channel: channels.Email, Destination: destination, Mode: mode})
	require.NoError(t, err)
	return d
}

func TestCheckEligibleByDefault(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.AcmeID, "fresh@example.com", eligibility.Marketing(eligibility.SourceBroadcasts))
	assert.True(t, d.Eligible)
	assert.Empty(t, d.Reason)
}

func TestCheckSuppressed(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.TyrellID, fixtures.SuppressionTyrellSuppressedDestination, eligibility.Marketing(eligibility.SourceBroadcasts))
	assert.False(t, d.Eligible)
	assert.Equal(t, skipreason.Suppressed, d.Reason)
}

// A lifted Suppression no longer blocks, in a single check or in the contact predicate.
func TestLiftedSuppressionDoesNotBlock(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.AcmeID, fixtures.ContactLiftedEmail, eligibility.Marketing(eligibility.SourceBroadcasts))
	assert.True(t, d.Eligible, "the fixture's lifted destination is mailable")
	assert.True(t, inAudience(t, env, fixtures.AcmeID, fixtures.ContactLiftedID, eligibility.Marketing(eligibility.SourceBroadcasts)),
		"an audience keeps the contact of a lifted destination")
}

// Transactional sends (source "") skip the unsubscribe and confirmation layers.
func TestCheckTransactionalSkipsUnsubscribe(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.TyrellID, fixtures.TyrellMatrixUnsubEverythingEmail, eligibility.Transactional())
	assert.True(t, d.Eligible, "transactional skips unsubscribe layers")
}

// A suppressed destination is never sent to, transactional or not, even when it also opted out.
func TestCheckTransactionalRespectsSuppression(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.TyrellID, fixtures.SuppressionTyrellTransactionalDestination, eligibility.Transactional())
	assert.False(t, d.Eligible)
	assert.Equal(t, skipreason.Suppressed, d.Reason)
}

func TestCheckUnsubscribedEverything(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.TyrellID, fixtures.TyrellMatrixUnsubEverythingEmail, eligibility.Marketing(eligibility.SourceBroadcasts))
	assert.Equal(t, skipreason.UnsubscribedEverything, d.Reason)
}

func TestCheckUnsubscribedFromOneSourceOnly(t *testing.T) {
	env := testhelper.Setup(t)
	src := eligibility.AutomationSource(7)

	assert.Equal(t, skipreason.UnsubscribedSource,
		decide(t, env, fixtures.TyrellID, fixtures.TyrellMatrixUnsubAutomationEmail, eligibility.Marketing(src)).Reason)
	assert.True(t, decide(t, env, fixtures.TyrellID, fixtures.TyrellMatrixUnsubAutomationEmail, eligibility.Marketing(eligibility.SourceBroadcasts)).Eligible,
		"eligible for a different source")
}

// The confirmation gate (ADR 0013) is a no-op when the workspace does not require it: an
// address with no Confirmation is still eligible, and the batch predicate still
// matches it. This is the "confirmed-opt-in off ⇒ behavior unchanged" invariant.
func TestConfirmationGateNoopWhenOff(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.TyrellID, fixtures.TyrellMatrixCleanEmail, eligibility.Marketing(eligibility.SourceBroadcasts))
	assert.True(t, d.Eligible, "no confirmation required when the gate is off")
	assert.True(t, inAudience(t, env, fixtures.TyrellID, fixtures.TyrellMatrixCleanID, eligibility.Marketing(eligibility.SourceBroadcasts)),
		"unconfirmed contact still in the audience when the gate is off")
}

// When the workspace requires confirmed opt-in, an unconfirmed address is blocked, via both
// the point Check and the batch Predicate.
func TestConfirmationGateBlocksUnconfirmed(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.LumonID, fixtures.LumonMatrixCleanEmail, eligibility.Marketing(eligibility.SourceBroadcasts))
	assert.False(t, d.Eligible)
	assert.Equal(t, skipreason.Unconfirmed, d.Reason)
	assert.False(t, inAudience(t, env, fixtures.LumonID, fixtures.LumonMatrixCleanID, eligibility.Marketing(eligibility.SourceBroadcasts)),
		"unconfirmed contact excluded from the audience")
}

// A confirmed address is mailable under the gate, via both the point Check and the Predicate.
func TestConfirmationGateAdmitsConfirmed(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.LumonID, fixtures.LumonMatrixConfirmedEmail, eligibility.Marketing(eligibility.SourceBroadcasts))
	assert.True(t, d.Eligible)
	assert.True(t, inAudience(t, env, fixtures.LumonID, fixtures.LumonMatrixConfirmedID, eligibility.Marketing(eligibility.SourceBroadcasts)),
		"confirmed contact included in the audience")
}

// Confirmation is the lowest-priority layer: a negative always dominates. A
// confirmed-but-suppressed address is still ineligible (and reports the negative).
func TestConfirmationDoesNotOverrideNegatives(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.LumonID, fixtures.LumonMatrixConfirmedSuppressedEmail, eligibility.Marketing(eligibility.SourceBroadcasts))
	assert.False(t, d.Eligible)
	assert.Equal(t, skipreason.Suppressed, d.Reason, "negatives dominate the confirmation")
}

// Stored destinations are lower-cased; an address typed in another case and padded still matches.
func TestCheckFoldsCase(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.TyrellID, "  "+strings.ToUpper(fixtures.TyrellMatrixSuppressedEmail)+"  ", eligibility.Marketing(eligibility.SourceBroadcasts))
	assert.False(t, d.Eligible, "input is normalized before lookup")
}

// A destination may have no Contact at all; Suppression still binds.
func TestCheckWorksWithoutAContact(t *testing.T) {
	env := testhelper.Setup(t)

	d := decide(t, env, fixtures.TyrellID, fixtures.SuppressionTyrellNoContactDestination, eligibility.Transactional())
	assert.Equal(t, skipreason.Suppressed, d.Reason)
}

// matrixEligibility returns the emails of a Workspace's matrix Contacts that the point Check
// admits and that the batch Predicate keeps, for one source.
func matrixEligibility(t *testing.T, env *testhelper.TestEnv, ws int64, mode eligibility.Mode) (byCheck, byPredicate []string) {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(ws)
	contacts, err := s.Contact().Query().All(ctx)
	require.NoError(t, err)
	for _, c := range contacts {
		d, err := eligibility.New().Check(ctx, s, eligibility.CheckInput{Channel: channels.Email, Destination: *c.Email, Mode: mode})
		require.NoError(t, err)
		if d.Eligible {
			byCheck = append(byCheck, *c.Email)
		}
	}
	kept, err := s.Contact().Query().Where(eligibility.Audience(eligibility.EmailDestination(), mode)).All(ctx)
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
		ws   int64
		mode eligibility.Mode
		want []string
	}{
		"gate off, transactional": {fixtures.TyrellID, eligibility.Transactional(), []string{
			fixtures.TyrellMatrixCleanEmail, fixtures.TyrellMatrixUnsubEverythingEmail, fixtures.TyrellMatrixUnsubBroadcastsEmail,
			fixtures.TyrellMatrixUnsubAutomationEmail, fixtures.TyrellMatrixConfirmedEmail,
		}},
		"gate off, broadcasts": {fixtures.TyrellID, eligibility.Marketing(eligibility.SourceBroadcasts), []string{
			fixtures.TyrellMatrixCleanEmail, fixtures.TyrellMatrixUnsubAutomationEmail, fixtures.TyrellMatrixConfirmedEmail,
		}},
		"gate off, automation": {fixtures.TyrellID, eligibility.Marketing(eligibility.AutomationSource(7)), []string{
			fixtures.TyrellMatrixCleanEmail, fixtures.TyrellMatrixUnsubBroadcastsEmail, fixtures.TyrellMatrixConfirmedEmail,
		}},
		"gate on, transactional": {fixtures.LumonID, eligibility.Transactional(), []string{
			fixtures.LumonMatrixCleanEmail, fixtures.LumonMatrixUnsubEverythingEmail, fixtures.LumonMatrixUnsubBroadcastsEmail,
			fixtures.LumonMatrixUnsubAutomationEmail, fixtures.LumonMatrixConfirmedEmail,
		}},
		"gate on, broadcasts": {fixtures.LumonID, eligibility.Marketing(eligibility.SourceBroadcasts), []string{fixtures.LumonMatrixConfirmedEmail}},
		"gate on, automation": {fixtures.LumonID, eligibility.Marketing(eligibility.AutomationSource(7)), []string{fixtures.LumonMatrixConfirmedEmail}},
	} {
		byCheck, byPredicate := matrixEligibility(t, env, tc.ws, tc.mode)
		assert.ElementsMatch(t, tc.want, byCheck, "%s: point Check", name)
		assert.ElementsMatch(t, tc.want, byPredicate, "%s: batch Predicate", name)
	}
}

// A Binding is the opt-in of a Telegram Destination: a Workspace that requires confirmed opt-in asks
// no Confirmation of it, as it does of an email address (ADR 0077).
func TestCheckOnTelegramAsksNoConfirmation(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	d, err := eligibility.New().Check(ctx, env.DB.Scoped(fixtures.LumonID), eligibility.CheckInput{Channel: channels.Telegram, Destination: "70001", Mode: eligibility.Marketing(eligibility.SourceBroadcasts)})

	require.NoError(t, err)
	assert.True(t, d.Eligible)
	assert.False(t, decide(t, env, fixtures.LumonID, "nobody@example.com", eligibility.Marketing(eligibility.SourceBroadcasts)).Eligible, "email there does ask for one")
}

// smsMatrix returns the matrix phones of a Workspace that the point Check on sms admits and the
// phones of the Contacts the batch Predicate keeps, for one source.
func smsMatrix(t *testing.T, env *testhelper.TestEnv, ws int64, phones []string, source string) (byCheck, byPredicate []string) {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(ws)
	for _, p := range phones {
		d, err := eligibility.New().Check(ctx, s, eligibility.CheckInput{Channel: channels.SMS, Destination: p, Mode: eligibility.Marketing(source)})
		require.NoError(t, err)
		if d.Eligible {
			byCheck = append(byCheck, p)
		}
	}
	kept, err := s.Contact().Query().Where(eligibility.Audience(eligibility.PhoneDestination(), eligibility.Marketing(source))).All(ctx)
	require.NoError(t, err)
	for _, c := range kept {
		byPredicate = append(byPredicate, *c.Phone)
	}
	return byCheck, byPredicate
}

// On sms a Confirmation is always required (ADR 0077): a number without one is not eligible whether
// the Workspace's confirmed-opt-in setting is on (Lumon) or off (Tyrell), one with a Confirmation is,
// and an opt-out of everything on sms blocks it. The point Check and the audience agree.
func TestSMSRequiresAConfirmationWhateverTheEmailSetting(t *testing.T) {
	env := testhelper.Setup(t)

	for name, tc := range map[string]struct {
		ws     int64
		phones []string
		want   []string
	}{
		"gate off": {fixtures.TyrellID, []string{fixtures.TyrellMatrixCleanPhone, fixtures.TyrellMatrixUnsubEverythingPhone, fixtures.TyrellMatrixConfirmedPhone},
			[]string{fixtures.TyrellMatrixConfirmedPhone}},
		"gate on": {fixtures.LumonID, []string{fixtures.LumonMatrixCleanPhone, fixtures.LumonMatrixUnsubEverythingPhone, fixtures.LumonMatrixConfirmedPhone},
			[]string{fixtures.LumonMatrixConfirmedPhone}},
	} {
		byCheck, byPredicate := smsMatrix(t, env, tc.ws, tc.phones, eligibility.SourceBroadcasts)
		assert.ElementsMatch(t, tc.want, byCheck, "%s: point Check", name)
		assert.ElementsMatch(t, tc.want, byPredicate, "%s: batch Predicate", name)
	}
}

// The reasons a number is not eligible on sms: no Confirmation, or an opt-out of everything.
func TestSMSReasons(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.TyrellID)

	unconfirmed, err := eligibility.New().Check(ctx, s, eligibility.CheckInput{Channel: channels.SMS, Destination: fixtures.TyrellMatrixCleanPhone, Mode: eligibility.Marketing(eligibility.SourceBroadcasts)})
	require.NoError(t, err)
	optedOut, err := eligibility.New().Check(ctx, s, eligibility.CheckInput{Channel: channels.SMS, Destination: fixtures.TyrellMatrixUnsubEverythingPhone, Mode: eligibility.Marketing(eligibility.AutomationSource(7))})
	require.NoError(t, err)

	assert.Equal(t, skipreason.Unconfirmed, unconfirmed.Reason)
	assert.Equal(t, skipreason.UnsubscribedEverything, optedOut.Reason)
}

// bindingAudience returns the Contacts of Lacuna that the batch Predicate keeps on a Channel of
// Bindings, reached through the Integration's Bindings, for one source.
func bindingAudience(t *testing.T, env *testhelper.TestEnv, integrationID int64, mode eligibility.Mode) []int64 {
	t.Helper()
	ids, err := env.DB.Scoped(fixtures.LacunaID).Contact().Query().
		Where(eligibility.Audience(eligibility.BindingDestination(channels.Telegram, integrationID), mode)).
		IDs(context.Background())
	require.NoError(t, err)
	return ids
}

// On a Channel of Bindings a Contact is reached at each of its own Bindings on the Integration, each
// one checked on its own (ADR 0075): Lacuna's bot reaches Tia (two chats), Ben, Fay and Eve, not
// Sid, whose one chat stopped Broadcasts, nor Cal, who is on the other bot only, nor the texters, who have no Binding at all. A transactional send ignores
// Sid's opt-out, and an Integration of 0 reaches nobody.
func TestBindingAudienceReachesTheContactsBoundOnTheIntegration(t *testing.T) {
	env := testhelper.Setup(t)

	for name, tc := range map[string]struct {
		integration int64
		mode        eligibility.Mode
		want        []int64
	}{
		"broadcasts on the bot": {fixtures.IntegrationLacunaTelegramID, eligibility.Marketing(eligibility.SourceBroadcasts), []int64{
			fixtures.ContactLacunaTwoChatsID, fixtures.ContactLacunaBenID, fixtures.ContactLacunaFayID, fixtures.ContactLacunaEveID,
		}},
		"transactional on the bot": {fixtures.IntegrationLacunaTelegramID, eligibility.Transactional(), []int64{
			fixtures.ContactLacunaTwoChatsID, fixtures.ContactLacunaBenID, fixtures.ContactLacunaFayID, fixtures.ContactLacunaEveID,
			fixtures.ContactLacunaStoppedID,
		}},
		"broadcasts on the other bot": {fixtures.IntegrationLacunaOtherTelegramID, eligibility.Marketing(eligibility.SourceBroadcasts), []int64{fixtures.ContactLacunaOtherBotID}},
		"no Integration":              {0, eligibility.Marketing(eligibility.SourceBroadcasts), nil},
	} {
		got := bindingAudience(t, env, tc.integration, tc.mode)

		assert.ElementsMatch(t, tc.want, got, name)
	}
}

// An erased Contact's opt-out on sms is kept under the hash of the number (ADR 0021), so it still
// refuses the number when it comes back, in the point Check and in the audience alike.
func TestSMSOptOutKeptUnderTheNumbersHashStillBlocks(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.TyrellID)
	hashed, err := s.Unsubscribe().Update().
		Where(unsubscribe.ChannelEQ(channels.SMS), unsubscribe.DestinationEQ(fixtures.TyrellMatrixUnsubEverythingPhone)).
		SetDestination(phone.Hash(fixtures.TyrellMatrixUnsubEverythingPhone)).
		Save(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, hashed)

	d, err := eligibility.New().Check(ctx, s, eligibility.CheckInput{Channel: channels.SMS, Destination: fixtures.TyrellMatrixUnsubEverythingPhone, Mode: eligibility.Marketing(eligibility.SourceBroadcasts)})
	require.NoError(t, err)
	byCheck, byPredicate := smsMatrix(t, env, fixtures.TyrellID,
		[]string{fixtures.TyrellMatrixUnsubEverythingPhone, fixtures.TyrellMatrixConfirmedPhone}, eligibility.SourceBroadcasts)

	assert.Equal(t, skipreason.UnsubscribedEverything, d.Reason)
	assert.Equal(t, []string{fixtures.TyrellMatrixConfirmedPhone}, byCheck)
	assert.Equal(t, []string{fixtures.TyrellMatrixConfirmedPhone}, byPredicate)
}

// A confirmation email is checked in the mode of who asked for it (ADR 0013): asked by the Workspace
// it respects Suppression and an opt-out of everything, asked by the person only Suppression. Neither
// asks for a Confirmation, which it is sent to obtain, nor reads a per-source opt-out. The point check
// and the audience agree, in a Workspace that requires confirmed opt-in.
func TestConfirmationModes(t *testing.T) {
	env := testhelper.Setup(t)

	cases := map[string]struct {
		mode eligibility.Mode
		want map[string]skipreason.Reason
	}{
		"asked by the Workspace": {eligibility.ConfirmationByWorkspace(), map[string]skipreason.Reason{
			fixtures.LumonMatrixCleanEmail:           "",
			fixtures.LumonMatrixUnsubBroadcastsEmail: "",
			fixtures.LumonMatrixUnsubEverythingEmail: skipreason.UnsubscribedEverything,
			fixtures.LumonMatrixSuppressedEmail:      skipreason.Suppressed,
		}},
		"asked by the person": {eligibility.ConfirmationByPerson(), map[string]skipreason.Reason{
			fixtures.LumonMatrixCleanEmail:           "",
			fixtures.LumonMatrixUnsubBroadcastsEmail: "",
			fixtures.LumonMatrixUnsubEverythingEmail: "",
			fixtures.LumonMatrixSuppressedEmail:      skipreason.Suppressed,
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for address, reason := range tc.want {
				wantEligible := reason == ""
				d := decide(t, env, fixtures.LumonID, address, tc.mode)
				kept, err := env.DB.Scoped(fixtures.LumonID).Contact().Query().
					Where(contact.Email(address), eligibility.Audience(eligibility.EmailDestination(), tc.mode)).
					Exist(t.Context())
				require.NoError(t, err)
				assert.Equal(t, reason, d.Reason, address)
				assert.Equal(t, wantEligible, d.Eligible, address)
				assert.Equal(t, wantEligible, kept, "the audience agrees for %s", address)
			}
		})
	}
}

// A Mode no constructor made names no rule: the check refuses it and the audience keeps nobody.
func TestZeroModeFailsClosed(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := eligibility.New().Check(t.Context(), env.DB.Scoped(fixtures.TyrellID),
		eligibility.CheckInput{Channel: channels.Email, Destination: fixtures.TyrellMatrixCleanEmail})
	require.Error(t, err)
	kept, err := env.DB.Scoped(fixtures.TyrellID).Contact().Query().
		Where(eligibility.Audience(eligibility.EmailDestination(), eligibility.Mode{})).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, kept)
}
