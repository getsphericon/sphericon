package outbound_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// perIntegration builds the sender of the Integration it is asked for, as the real resolver does,
// and none for an Integration it does not know.
type perIntegration map[int64]messaging.EmailSender

func (p perIntegration) EmailSender(_ context.Context, _ *ent.Scoped, integ *ent.Integration) (messaging.EmailSender, error) {
	if integ == nil {
		return nil, messaging.ErrNoProvider
	}
	sender, ok := p[integ.ID]
	if !ok {
		return nil, messaging.ErrNoProvider
	}
	return sender, nil
}

// broadcastTestVia is a Test send of a Broadcast of Acme, which has a verified Sending domain and
// the SMTP default, to its owner.
func broadcastTestVia(env *testhelper.TestEnv, via perIntegration, integrationID *int64) outbound.TestReport {
	return broadcastTestTo(env, fixtures.AcmeID, "hello@codebasics.dev", fixtures.OwnerJaneEmail, via, integrationID)
}

// globexBroadcastTestVia is the same for Globex, which has no verified Sending domain and no default.
func globexBroadcastTestVia(env *testhelper.TestEnv, to string, via perIntegration, integrationID *int64) outbound.TestReport {
	return broadcastTestTo(env, fixtures.GlobexID, "hello@globex.test", to, via, integrationID)
}

func broadcastTestTo(env *testhelper.TestEnv, workspaceID int64, from, to string, via perIntegration, integrationID *int64) outbound.TestReport {
	m := outbound.New(env.Bus, env.Holds, via, tracking.New("test-secret", "http://local"),
		outbound.WithSystemSender(env.SystemMail))
	b := &ent.Broadcast{WorkspaceID: workspaceID, Subject: "Launch", Body: htmlBody, FromEmail: &from, IntegrationID: integrationID}
	return m.SendBroadcastTest(context.Background(), env.DB.Scoped(workspaceID), b, to,
		&outbound.Asker{UserID: fixtures.OwnerJaneID, Email: fixtures.OwnerJaneEmail})
}

// missingIntegrationID names no Integration of any Workspace.
const missingIntegrationID = 987654

func idOf(id int) *int64 { v := int64(id); return &v }

func TestBroadcastTestSendGoesThroughTheBroadcastsOwnIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	own, byDefault := &testhelper.CapturingSender{}, &testhelper.CapturingSender{}
	via := perIntegration{fixtures.IntegrationAcmePostmarkID: own, fixtures.IntegrationAcmeDefaultID: byDefault}

	report := broadcastTestVia(env, via, idOf(fixtures.IntegrationAcmePostmarkID))

	require.Empty(t, report.Detail)
	assert.Equal(t, outbound.Sent, report.Outcome)
	assert.Len(t, own.Messages(), 1, "the Test send is the mail the Broadcast will send")
	assert.Empty(t, byDefault.Messages())
}

func TestBroadcastTestSendWithoutAnIntegrationGoesThroughTheDefault(t *testing.T) {
	env := testhelper.Setup(t)
	own, byDefault := &testhelper.CapturingSender{}, &testhelper.CapturingSender{}
	via := perIntegration{fixtures.IntegrationAcmePostmarkID: own, fixtures.IntegrationAcmeDefaultID: byDefault}

	report := broadcastTestVia(env, via, nil)

	require.Empty(t, report.Detail)
	assert.Len(t, byDefault.Messages(), 1)
	assert.Empty(t, own.Messages())
}

func TestBroadcastTestSendThroughATestInboxOfTheBroadcastIsCapturedWhateverTheDefaultIs(t *testing.T) {
	env := testhelper.Setup(t)
	kept := &keepingSender{}
	via := perIntegration{fixtures.IntegrationAcmePostmarkID: kept, fixtures.IntegrationAcmeDefaultID: env.CustomerMail}

	report := broadcastTestVia(env, via, idOf(fixtures.IntegrationAcmePostmarkID))

	require.Empty(t, report.Detail)
	assert.True(t, report.Captured)
	assert.Len(t, kept.sent, 1)
	assert.Empty(t, env.CustomerMail.Messages())
}

func TestBroadcastTestSendThroughARealIntegrationOfTheBroadcastIsNotCapturedWhateverTheDefaultIs(t *testing.T) {
	env := testhelper.Setup(t)
	kept := &keepingSender{}
	via := perIntegration{fixtures.IntegrationAcmePostmarkID: env.CustomerMail, fixtures.IntegrationAcmeDefaultID: kept}

	report := broadcastTestVia(env, via, idOf(fixtures.IntegrationAcmePostmarkID))

	require.Empty(t, report.Detail)
	assert.False(t, report.Captured)
	assert.Len(t, env.CustomerMail.Messages(), 1)
	assert.Empty(t, kept.sent, "the default Test inbox does not take the Test send of another Integration")
}

func TestBroadcastTestSendOfAnIntegrationThatCannotSendIsHeldNotSentThroughTheDefault(t *testing.T) {
	cases := map[string]*int64{
		"disabled": idOf(fixtures.IntegrationAcmeSesID),
		"archived": idOf(fixtures.IntegrationAcmeArchivedID),
		"unknown":  idOf(missingIntegrationID),
	}
	for name, id := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			via := perIntegration{fixtures.IntegrationAcmeDefaultID: env.CustomerMail}

			report := broadcastTestVia(env, via, id)

			assert.Equal(t, outbound.Held, report.Outcome)
			assert.Equal(t, outbound.HoldNoIntegration, report.Reason)
			assert.Empty(t, env.CustomerMail.Messages())
			assert.Empty(t, env.SystemMail.Messages())
		})
	}
}

func TestBroadcastTestSendWithoutAVerifiedDomainKeepsTheSystemSenderForARealIntegrationOfTheBroadcast(t *testing.T) {
	env := testhelper.Setup(t)
	via := perIntegration{fixtures.IntegrationGlobexID: env.CustomerMail}
	own := idOf(fixtures.IntegrationGlobexID)

	report := globexBroadcastTestVia(env, fixtures.OwnerJaneEmail, via, own)
	require.Empty(t, report.Detail)
	assert.False(t, report.Captured)
	assert.Len(t, env.SystemMail.Messages(), 1)
	assert.Empty(t, env.CustomerMail.Messages())

	other := globexBroadcastTestVia(env, "stranger@example.com", via, own)
	assert.Equal(t, outbound.Refused, other.Outcome, "only the asker's own address")
	assert.Equal(t, outbound.RefuseOtherRecipient, other.Reason)
}

func TestBroadcastTestSendWithoutAVerifiedDomainIsCapturedByATestInboxOfTheBroadcast(t *testing.T) {
	env := testhelper.Setup(t)
	kept := &keepingSender{}
	via := perIntegration{fixtures.IntegrationGlobexInboxID: kept}

	report := globexBroadcastTestVia(env, "someone-else@example.com", via, idOf(fixtures.IntegrationGlobexInboxID))

	require.Empty(t, report.Detail)
	assert.True(t, report.Captured)
	assert.Len(t, kept.sent, 1, "to the address named, not only the asker's")
	assert.Empty(t, env.SystemMail.Messages())
}

func TestBroadcastTestSendWithoutAVerifiedDomainDoesNotFallBackToTheSystemSenderForAnIntegrationThatCannotSend(t *testing.T) {
	env := testhelper.Setup(t)

	report := globexBroadcastTestVia(env, fixtures.OwnerJaneEmail, perIntegration{}, idOf(missingIntegrationID))

	assert.Equal(t, outbound.Held, report.Outcome)
	assert.Equal(t, outbound.HoldNoIntegration, report.Reason)
	assert.Empty(t, env.SystemMail.Messages())
}
