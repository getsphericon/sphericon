package contacterasure_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/hook"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/ent/trackedlink"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/contacterasure"
)

// The Initech Workspace holds the fixture Contact that carries a row in every table that can
// depend on one (ContactErasable) and a bystander next to it (ContactBystander).

// erase erases what id names in the Workspace, as an API token asks for it.
func erase(t *testing.T, env *testhelper.TestEnv, workspaceID int64, id contacterasure.Identifier) error {
	t.Helper()
	return contacterasure.New(env.Bus, testhelper.Module[*broadcasts.Module](env), testhelper.Module[*contacts.Module](env), testhelper.Module[*unsubscribes.Module](env), time.Now).Erase(t.Context(), env.DB.Scoped(workspaceID), id, apiRequester)
}

// eraseErasable erases ContactErasable by its id.
func eraseErasable(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	require.NoError(t, erase(t, env, fixtures.InitechID, contacterasure.Identifier{ContactID: fixtures.ContactErasableID}))
}

func contactExists(t *testing.T, env *testhelper.TestEnv, id int64) bool {
	t.Helper()
	return env.DB.Contact.Query().Where(contact.ID(id)).ExistX(t.Context())
}

func eventExists(t *testing.T, env *testhelper.TestEnv, id int64) bool {
	t.Helper()
	return env.DB.Event.Query().Where(event.ID(id)).ExistX(t.Context())
}

// sha256Hex is how a consumer of the Erasures stream names an address or a number: the lowercase
// hex of its SHA-256.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestAnErasureRemovesThePersonsDataAndKeepsTheFactsAnonymous(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	// An Enrollment's copy of the Event it began on (#752) is of the Contact's runs alone.
	copies := env.DB.AutomationRun.Query().Where(automationrun.WorkspaceID(fixtures.InitechID), automationrun.EventPropertiesNotNil())
	require.True(t, copies.Clone().ExistX(ctx), "the erasable run holds a copy of its triggering Event")

	eraseErasable(t, env)

	assert.False(t, contactExists(t, env, fixtures.ContactErasableID))
	assert.False(t, copies.Clone().ExistX(ctx), "no copy of the triggering Event's properties is left")
	// Removed outright: its Visitors, Tag links, Confirmation and runs.
	assert.False(t, env.DB.Visitor.Query().Where(visitor.VisitorID(fixtures.VisitorErasableVisitorID)).ExistX(ctx), "visitors")
	linked := env.DB.Tag.Query().Where(tag.ID(fixtures.TagInitechID)).QueryContacts().IDsX(ctx)
	assert.Equal(t, []int64{fixtures.ContactBystanderID}, linked, "the tag stays, only the bystander keeps it")
	assert.False(t, env.DB.Confirmation.Query().Where(confirmation.ID(fixtures.ConfirmationErasableID)).ExistX(ctx), "confirmation")
	assert.False(t, env.DB.AutomationRun.Query().Where(automationrun.ID(fixtures.AutomationRunErasableID)).ExistX(ctx), "automation runs")

	// Events: customer-tracked and marketing.confirmed are deleted ...
	for _, id := range []int64{
		fixtures.EventErasableCustomID, fixtures.EventErasableAnonymousID, fixtures.EventErasableConfirmedID,
	} {
		assert.Falsef(t, eventExists(t, env, id), "event %d is deleted", id)
	}
	// ... every reserved system Event stays as an anonymous row.
	for _, id := range []int64{
		fixtures.EventErasableSentID, fixtures.EventErasableOpenedID, fixtures.EventErasableClickedID,
		fixtures.EventErasableBouncedID, fixtures.EventErasableComplainedID, fixtures.EventErasableUnsubscribedID,
		fixtures.EventErasableCreatedID, fixtures.EventErasableDeliveredID, fixtures.EventErasableDelayedID,
		fixtures.EventErasableDroppedID,
	} {
		e := env.DB.Event.GetX(ctx, id)
		assert.Nilf(t, e.ContactID, "event %d contact_id", id)
		assert.Nilf(t, e.VisitorID, "event %d visitor_id", id)
		assert.Nilf(t, e.Email, "event %d email", id)
		assert.Nilf(t, e.Phone, "event %d phone", id)
		assert.Emptyf(t, e.SubjectID, "event %d subject_id", id)
		assert.NotNilf(t, e.OccurredAt, "event %d keeps its time", id)
	}
	// Only the non-personal delivery facts the rates are computed from survive.
	assert.Equal(t, map[string]any{"sendingDomain": fixtures.SendingDomainInitechDomain}, env.DB.Event.GetX(ctx, fixtures.EventErasableSentID).Properties)
	assert.Equal(t, map[string]any{"sendingDomain": fixtures.SendingDomainInitechDomain, "bounceKind": "hard"}, env.DB.Event.GetX(ctx, fixtures.EventErasableBouncedID).Properties)
	clicked := env.DB.Event.GetX(ctx, fixtures.EventErasableClickedID)
	assert.Empty(t, clicked.Properties, "the clicked url is gone")
	assert.Nil(t, clicked.URL, "and so is its column")
	assert.Nil(t, clicked.BroadcastID, "the provenance goes with the properties")
	assert.Nil(t, env.DB.Event.GetX(ctx, fixtures.EventErasableOpenedID).BroadcastID)
	assert.Empty(t, env.DB.Event.GetX(ctx, fixtures.EventErasableCreatedID).Properties)

	// The refusal survives by Destination, detached from the Contact.
	unsub := env.DB.Unsubscribe.GetX(ctx, fixtures.UnsubscribeErasableID)
	assert.Equal(t, fixtures.ContactErasableEmail, unsub.Destination)
	assert.Nil(t, unsub.ContactID)
	supp := env.DB.Suppression.GetX(ctx, fixtures.SuppressionErasableID)
	assert.Equal(t, fixtures.ContactErasableEmail, supp.Destination)
	assert.Nil(t, supp.ContactID)
	// A lifted Suppression is no refusal to honour any more: Erasure deletes it.
	lifted := env.DB.Suppression.Query().Where(suppression.ID(fixtures.SuppressionErasableLiftedID)).ExistX(ctx)
	assert.False(t, lifted, "the lifted Suppression of the Contact is deleted")

	// Delivery records stay as rows that no longer name anyone.
	for _, id := range []int64{fixtures.OutboundMessageErasableBroadcastID, fixtures.OutboundMessageErasableTransactionalID} {
		m := env.DB.OutboundMessage.GetX(ctx, id)
		assert.Emptyf(t, m.Destination, "message %d destination", id)
		assert.Nilf(t, m.ContactID, "message %d contact_id", id)
		assert.Nilf(t, m.Subject, "message %d subject: what the Contact received goes with the Contact", id)
		assert.Equal(t, outboundmessage.StatusSent, m.Status)
	}
	assert.Zero(t, env.DB.BroadcastRecipient.GetX(ctx, fixtures.BroadcastRecipientErasableID).ContactID)
	// The Enrollment is deleted; the message it sent keeps its Automation and loses the run.
	sent := env.DB.OutboundMessage.GetX(ctx, fixtures.OutboundMessageErasableAutomationID)
	assert.Nil(t, sent.AutomationRunID)
	assert.EqualValues(t, fixtures.AutomationInitechID, *sent.AutomationID)
}

func TestAnErasureLeavesOtherContactsAlone(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	eraseErasable(t, env)

	b := env.DB.Contact.GetX(ctx, fixtures.ContactBystanderID)
	assert.Equal(t, fixtures.ContactBystanderEmail, *b.Email)
	for _, id := range []int64{fixtures.EventBystanderCustomID, fixtures.EventBystanderSentID} {
		e := env.DB.Event.GetX(ctx, id)
		assert.EqualValues(t, fixtures.ContactBystanderID, *e.ContactID)
		assert.Equal(t, fixtures.ContactBystanderEmail, *e.Email)
	}
	assert.Equal(t, fixtures.ContactBystanderEmail, env.DB.OutboundMessage.GetX(ctx, fixtures.OutboundMessageBystanderID).Destination)
	assert.EqualValues(t, fixtures.ContactBystanderID, env.DB.BroadcastRecipient.GetX(ctx, fixtures.BroadcastRecipientBystanderID).ContactID)
}

// A failure of the last step, deleting the Contact, comes after every other step wrote: the whole
// Erasure rolls back with it, the work it cancelled included.
func TestAnErasureIsAllOrNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	errDown := errors.New("storage down")
	isContact := func(_ context.Context, m ent.Mutation) bool { return m.Type() == ent.TypeContact }
	deletesAContact := hook.And(isContact, hook.HasOp(ent.OpDelete|ent.OpDeleteOne))
	env.Bus.Use(hook.If(hook.FixedError(errDown), deletesAContact))
	env.EnqueueJob(t, jobs.EvaluateTriggerArgs{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID, Action: "x"})
	env.EnqueueJob(t, jobs.EvaluateRulesArgs{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID, Action: "rule_initech", SourceID: "evt-1"})
	env.EnqueueJob(t, automations.RunHTTPStepArgs{WorkspaceID: fixtures.InitechID, RunID: fixtures.AutomationRunErasableID})

	err := erase(t, env, fixtures.InitechID, contacterasure.Identifier{ContactID: fixtures.ContactErasableID})

	require.ErrorIs(t, err, errDown)
	assert.True(t, contactExists(t, env, fixtures.ContactErasableID))
	sent := env.DB.Event.GetX(ctx, fixtures.EventErasableSentID)
	assert.EqualValues(t, fixtures.ContactErasableID, *sent.ContactID)
	assert.Equal(t, fixtures.ContactErasableEmail, *sent.Email)
	assert.True(t, eventExists(t, env, fixtures.EventErasableCustomID))
	assert.True(t, eventExists(t, env, fixtures.EventErasableConfirmedID))
	assert.Equal(t, fixtures.ContactErasableEmail, env.DB.OutboundMessage.GetX(ctx, fixtures.OutboundMessageErasableBroadcastID).Destination)
	assert.NotNil(t, env.DB.Unsubscribe.GetX(ctx, fixtures.UnsubscribeErasableID).ContactID)
	assert.True(t, env.DB.Visitor.Query().Where(visitor.VisitorID(fixtures.VisitorErasableVisitorID)).ExistX(ctx))
	assert.True(t, env.DB.Confirmation.Query().Where(confirmation.ID(fixtures.ConfirmationErasableID)).ExistX(ctx))
	assert.Equal(t, 2, env.DB.Suppression.Query().Where(suppression.ContactID(fixtures.ContactErasableID)).CountX(ctx), "the active and the lifted Suppression")
	assert.True(t, env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.ID(fixtures.BroadcastRecipientErasablePendingID)).ExistX(ctx), "the unsent recipient")
	assert.Equal(t, broadcast.StatusSending, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastInitechSendingSoloID).Status, "the Broadcast is not settled")
	assert.True(t, env.DB.AutomationRun.Query().Where(automationrun.ID(fixtures.AutomationRunErasableID)).ExistX(ctx), "the Enrollment")
	assert.True(t, env.DB.RuleFiring.Query().Where(rulefiring.ContactID(fixtures.ContactErasableID)).ExistX(ctx), "the Rule firing")
	assert.Equal(t, 1, env.DB.Unsubscribe.Query().Where(unsubscribe.ContactID(fixtures.ContactErasableID)).CountX(ctx))
	assert.Len(t, env.JobsOf(t, jobs.EvaluateTriggerArgs{}.Kind()), 1, "the queued trigger evaluation")
	assert.Len(t, env.JobsOf(t, jobkind.RuleEvaluate.Kind), 1, "the queued Rule evaluation")
	assert.Len(t, env.JobsOf(t, jobkind.HTTPStep.Kind), 1, "the queued HTTP step call")
	assert.Empty(t, env.QueuedEnvelopes(t, systemevent.ContactErased), "no signal")
}

// An identifier that resolves to nothing in the Workspace erases nothing and says so, whatever
// another Workspace holds under it.
func TestAnErasureOfWhatTheWorkspaceDoesNotHoldIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	email := func(s string) contacterasure.Identifier { return contacterasure.Identifier{Email: &s} }
	visitorID := func(s string) contacterasure.Identifier { return contacterasure.Identifier{VisitorID: &s} }

	unknownID := erase(t, env, fixtures.InitechID, contacterasure.Identifier{ContactID: 999999})
	unknownEmail := erase(t, env, fixtures.InitechID, email("nobody@initech.test"))
	unknownVisitor := erase(t, env, fixtures.InitechID, visitorID("vis-nobody"))
	foreignID := erase(t, env, fixtures.AcmeID, contacterasure.Identifier{ContactID: fixtures.ContactErasableID})
	foreignEmail := erase(t, env, fixtures.AcmeID, email(fixtures.ContactErasableEmail))
	foreignVisitor := erase(t, env, fixtures.AcmeID, visitorID(fixtures.VisitorErasableVisitorID))
	foreignAnonymous := erase(t, env, fixtures.AcmeID, visitorID(fixtures.VisitorAnonymousVisitorID))

	require.ErrorIs(t, unknownID, contacterasure.ErrContactNotFound)
	require.ErrorIs(t, unknownEmail, contacterasure.ErrNothingFound)
	require.ErrorIs(t, unknownVisitor, contacterasure.ErrNothingFound)
	require.ErrorIs(t, foreignID, contacterasure.ErrContactNotFound)
	require.ErrorIs(t, foreignEmail, contacterasure.ErrNothingFound)
	require.ErrorIs(t, foreignVisitor, contacterasure.ErrNothingFound)
	require.ErrorIs(t, foreignAnonymous, contacterasure.ErrNothingFound)
	assert.True(t, contactExists(t, env, fixtures.ContactErasableID))
	assert.Equal(t, fixtures.ContactErasableEmail, *env.DB.Event.GetX(ctx, fixtures.EventErasableSentID).Email)
	assert.True(t, eventExists(t, env, fixtures.EventAnonymousVisitorID))
	assert.Empty(t, env.QueuedEnvelopes(t, systemevent.ContactErased), "a refused Erasure emits nothing")
}

// An address erases its Contact by the rules of an id, whatever its padding and case.
func TestAnErasureByAnAddressErasesItsContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	address := " Erin@Initech.TEST "

	require.NoError(t, erase(t, env, fixtures.InitechID, contacterasure.Identifier{Email: &address}))

	assert.False(t, contactExists(t, env, fixtures.ContactErasableID))
	assert.False(t, eventExists(t, env, fixtures.EventErasableCustomID))
	sent := env.DB.Event.GetX(ctx, fixtures.EventErasableSentID)
	assert.Nil(t, sent.ContactID)
	assert.Nil(t, sent.Email)
	unsub := env.DB.Unsubscribe.GetX(ctx, fixtures.UnsubscribeErasableID)
	assert.Equal(t, fixtures.ContactErasableEmail, unsub.Destination)
	assert.Nil(t, unsub.ContactID)
	assert.Empty(t, env.DB.OutboundMessage.GetX(ctx, fixtures.OutboundMessageErasableTransactionalID).Destination)
	assert.Equal(t, fixtures.ContactBystanderEmail, env.DB.OutboundMessage.GetX(ctx, fixtures.OutboundMessageBystanderID).Destination)
}

// An address with delivery records and no Contact is a subject of its own: its records stay for the
// totals and no longer name it.
func TestAnErasureByAnAddressWithoutAContactAnonymizesItsDeliveryRecords(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	address := fixtures.OutboundMessageStrangerDestination

	require.NoError(t, erase(t, env, fixtures.InitechID, contacterasure.Identifier{Email: &address}))

	m := env.DB.OutboundMessage.GetX(ctx, fixtures.OutboundMessageStrangerID)
	assert.Empty(t, m.Destination)
	assert.Equal(t, outboundmessage.StatusSent, m.Status, "the row stays for the totals")
	assert.Equal(t, fixtures.ContactBystanderEmail, env.DB.OutboundMessage.GetX(ctx, fixtures.OutboundMessageBystanderID).Destination)
}

// An address held only by a Captured message is a subject, and its Erasure takes that message and
// no other.
func TestAnErasureByAnAddressHeldOnlyByACapturedMessageDeletesIt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	address := fixtures.CapturedMessageGlobexWelcomeRecipient

	require.NoError(t, erase(t, env, fixtures.GlobexID, contacterasure.Identifier{Email: &address}))

	assert.False(t, env.DB.CapturedMessage.Query().Where(capturedmessage.ID(fixtures.CapturedMessageGlobexWelcomeID)).ExistX(ctx))
	assert.True(t, env.DB.CapturedMessage.Query().Where(capturedmessage.ID(fixtures.CapturedMessageGlobexNewsletterID)).ExistX(ctx), "another address's message stays")
}

// The Captured messages addressed to a Contact go with it, also those kept by an archived Test inbox,
// and another Workspace's messages stay.
func TestAnErasureOfAContactDeletesTheCapturedMessagesToItsAddress(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	globex := env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(ctx)
	require.NotZero(t, env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.AcmeID)).CountX(ctx))

	require.NoError(t, erase(t, env, fixtures.AcmeID, contacterasure.Identifier{ContactID: fixtures.ContactExportSubjectID}))

	assert.Zero(t, env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.AcmeID)).CountX(ctx))
	assert.Equal(t, globex, env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceID(fixtures.GlobexID)).CountX(ctx))
}

// A visitor id of an anonymous Visitor names that Visitor and its Events; a Contact's device stays.
func TestAnErasureByAnAnonymousVisitorDeletesItAndItsEvents(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	id := fixtures.VisitorAnonymousVisitorID

	require.NoError(t, erase(t, env, fixtures.InitechID, contacterasure.Identifier{VisitorID: &id}))

	assert.False(t, env.DB.Visitor.Query().Where(visitor.ID(fixtures.VisitorAnonymousID)).ExistX(ctx))
	assert.False(t, eventExists(t, env, fixtures.EventAnonymousVisitorID))
	assert.True(t, env.DB.Visitor.Query().Where(visitor.VisitorID(fixtures.VisitorErasableVisitorID)).ExistX(ctx), "a Contact's device is untouched")
	assert.True(t, contactExists(t, env, fixtures.ContactErasableID))
}

// A visitor bound to a Contact erases that Contact, by the same rules as by id.
func TestAnErasureByAVisitorOfAContactErasesTheContact(t *testing.T) {
	env := testhelper.Setup(t)
	id := fixtures.VisitorErasableVisitorID

	require.NoError(t, erase(t, env, fixtures.InitechID, contacterasure.Identifier{VisitorID: &id}))

	assert.False(t, contactExists(t, env, fixtures.ContactErasableID))
	assert.False(t, eventExists(t, env, fixtures.EventErasableAnonymousID))
}

// The short links of the Contact's messages go: following one would record a click of the person.
// Fixtures: TrackedLinkAcme is a link of Liam's welcome message.
func TestAnErasureDeletesTheShortLinksOfTheContactsMessages(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	require.NoError(t, erase(t, env, fixtures.AcmeID, contacterasure.Identifier{ContactID: fixtures.ContactLiamID}))

	assert.False(t, env.DB.TrackedLink.Query().Where(trackedlink.ID(fixtures.TrackedLinkAcmeID)).ExistX(ctx))
	assert.True(t, env.DB.TrackedLink.Query().Where(trackedlink.ID(fixtures.TrackedLinkGlobexID)).ExistX(ctx), "another Workspace's link stays")
	m := env.DB.OutboundMessage.GetX(ctx, fixtures.OutboundMessageWelcomeID)
	assert.Empty(t, m.Destination, "the message stays as an anonymous row")
	assert.Nil(t, m.ContactID)
}

// An opt-out of a number keeps refusing it after the Erasure, keyed by the number's hash and never
// by the number (ADR 0021). Fixtures: ContactTexterOptedOut opted out of everything on sms.
func TestAnErasureKeepsTheOptOutOfANumberByItsHash(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	require.NoError(t, erase(t, env, fixtures.AcmeID, contacterasure.Identifier{ContactID: fixtures.ContactTexterOptedOutID}))

	optOut := env.DB.Unsubscribe.GetX(ctx, fixtures.UnsubscribeTexterOptedOutID)
	assert.Equal(t, sha256Hex(fixtures.ContactTexterOptedOutPhone), optOut.Destination)
	assert.Nil(t, optOut.ContactID)
	named := env.DB.Unsubscribe.Query().Where(unsubscribe.Destination(fixtures.ContactTexterOptedOutPhone)).ExistX(ctx)
	assert.False(t, named, "no opt-out names the number")
	assert.False(t, env.DB.Confirmation.Query().Where(confirmation.ID(fixtures.ConfirmationTexterOptedOutID)).ExistX(ctx), "its Confirmation goes")
}

// An Erasure leaves one contact.erased signal (ADR 0021): the message names the Workspace, the
// requester and the identifier kind and carries the customer's subject_id and the hashes of the
// address and the number, never them; the stored Event holds no PII and no Contact reference.
func TestAnErasureEmitsOneContactErasedThatNamesNobody(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	eraseErasable(t, env)

	envelopes := env.QueuedEnvelopes(t, systemevent.ContactErased)
	require.Len(t, envelopes, 1, "once per Erasure")
	assert.EqualValues(t, fixtures.InitechID, envelopes[0].WorkspaceID)
	body := string(envelopes[0].Data)
	assert.NotContains(t, body, fixtures.ContactErasableEmail)
	assert.NotContains(t, body, fixtures.ContactErasablePhone[1:], "nor the number, in any form")
	var data map[string]any
	require.NoError(t, json.Unmarshal(envelopes[0].Data, &data))
	assert.Equal(t, "user-erin-300", data["subjectId"], "the customer's own id, for their downstream erasure")
	assert.Equal(t, "contact_id", data["identifierKind"])
	assert.Equal(t, "api_token", data["requesterKind"])
	assert.EqualValues(t, apiRequester.ID, data["requesterId"])
	assert.Contains(t, body, sha256Hex(fixtures.ContactErasableEmail))
	assert.Contains(t, body, sha256Hex(fixtures.ContactErasablePhone))

	stored := env.DB.Event.Query().Where(event.Action(systemevent.ContactErased)).OnlyX(ctx)
	assert.EqualValues(t, fixtures.InitechID, stored.WorkspaceID)
	assert.Nil(t, stored.ContactID)
	assert.Nil(t, stored.VisitorID)
	assert.Nil(t, stored.Email)
	assert.Nil(t, stored.Phone)
	assert.Empty(t, stored.SubjectID)
	assert.NotNil(t, stored.OccurredAt)
	assert.NotContains(t, stored.Properties, "subjectId")
	assert.Equal(t, "contact_id", stored.Properties["identifierKind"])
	assert.Equal(t, "api_token", stored.Properties["requesterKind"])
}

// A Contact with no subject_id is named downstream by its sphericon id.
func TestAnErasureNamesAContactWithoutASubjectIDByItsID(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, erase(t, env, fixtures.InitechID, contacterasure.Identifier{ContactID: fixtures.ContactBystanderID}))

	msgs := env.QueuedMessages(t, systemevent.ContactErased)
	require.Len(t, msgs, 1)
	assert.Equal(t, testhelper.ID(fixtures.ContactBystanderID), msgs[0].Data["subjectId"], "the fallback subject is the Contact's public id")
}

// An address alone is no subject id: the message names it by its hash only.
func TestAnErasureByAnAddressAloneNeverEchoesTheAddress(t *testing.T) {
	env := testhelper.Setup(t)
	address := fixtures.OutboundMessageStrangerDestination

	require.NoError(t, erase(t, env, fixtures.InitechID, contacterasure.Identifier{Email: &address}))

	envelopes := env.QueuedEnvelopes(t, systemevent.ContactErased)
	require.Len(t, envelopes, 1)
	assert.NotContains(t, string(envelopes[0].Data), fixtures.OutboundMessageStrangerDestination)
	var data map[string]any
	require.NoError(t, json.Unmarshal(envelopes[0].Data, &data))
	assert.Empty(t, data["subjectId"])
	assert.Equal(t, "email", data["identifierKind"])
}

// An active Suppression of a number keeps refusing it after the Erasure, keyed by the number's hash.
// Fixtures: ContactLacunaSuppressed's number is suppressed on sms.
func TestAnErasureKeepsTheSuppressionOfANumberByItsHash(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	require.NoError(t, erase(t, env, fixtures.LacunaID, contacterasure.Identifier{ContactID: fixtures.ContactLacunaSuppressedID}))

	kept := env.DB.Suppression.GetX(ctx, fixtures.SuppressionLacunaSalID)
	assert.Equal(t, sha256Hex(fixtures.ContactLacunaSuppressedPhone), kept.Destination)
	assert.Nil(t, kept.ContactID)
	assert.Nil(t, kept.LiftedAt, "it still refuses")
	named := env.DB.Suppression.Query().Where(suppression.Destination(fixtures.ContactLacunaSuppressedPhone)).ExistX(ctx)
	assert.False(t, named, "no Suppression names the number")
}
