package mcpserver_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An agent records a refusal of tracking over MCP, and cannot widen it again: a grant over the
// refusal is a conflict, as on /api (ADR 0121).
func TestMCPRecordsARefusalOfTrackingAndRefusesAGrantOverIt(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead))
	observation := func(state string) map[string]any {
		return map[string]any{"channel": "email", "destination": fixtures.ContactAliceEmail, "state": state}
	}

	denied := call(t, s, "engagement_tracking_create", observation("denied"))
	granted := call(t, s, "engagement_tracking_create", observation("granted"))
	read := call(t, s, "engagement_tracking_get_for_contact", map[string]any{"contactId": testhelper.ID(fixtures.ContactAliceID)})

	require.False(t, denied.IsError, text(t, denied))
	assert.True(t, granted.IsError)
	assert.Contains(t, text(t, granted), "HTTP 409 Conflict")
	assert.Contains(t, text(t, granted), "only the person can lift a refusal")
	require.False(t, read.IsError, text(t, read))
	assert.Contains(t, text(t, read), `"state":"denied"`)
}

// A Connection without contacts:write cannot record a consent.
func TestMCPRecordingTrackingConsentNeedsContactsWrite(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeContactsRead))

	res := call(t, s, "engagement_tracking_create", map[string]any{
		"channel": "email", "destination": fixtures.ContactAliceEmail, "state": "denied",
	})
	read := call(t, s, "engagement_tracking_get_for_contact", map[string]any{"contactId": testhelper.ID(fixtures.ContactAliceID)})

	assert.True(t, res.IsError)
	assert.Contains(t, text(t, res), "contacts:write")
	assert.Contains(t, text(t, read), `"state":"default"`)
}

// An agent records a refusal of a phone and of a Telegram chat over MCP, and a grant over either
// is a conflict.
func TestMCPRecordsARefusalOfTrackingOfAPhoneAndAChat(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead))
	onSMS := func(state string) map[string]any {
		return map[string]any{"channel": "sms", "destination": fixtures.ContactTexterPhone, "state": state}
	}
	onTelegram := func(state string) map[string]any {
		return map[string]any{"channel": "telegram", "destination": fixtures.BindingLiamDestination, "state": state}
	}

	smsDenied := call(t, s, "engagement_tracking_create", onSMS("denied"))
	chatDenied := call(t, s, "engagement_tracking_create", onTelegram("denied"))
	smsGranted := call(t, s, "engagement_tracking_create", onSMS("granted"))
	chatGranted := call(t, s, "engagement_tracking_create", onTelegram("granted"))
	read := call(t, s, "engagement_tracking_get_for_contact", map[string]any{"contactId": testhelper.ID(fixtures.ContactTexterID)})

	require.False(t, smsDenied.IsError, text(t, smsDenied))
	require.False(t, chatDenied.IsError, text(t, chatDenied))
	assert.Contains(t, text(t, smsGranted), "HTTP 409 Conflict")
	assert.Contains(t, text(t, chatGranted), "HTTP 409 Conflict")
	assert.Contains(t, text(t, read), `"destination":"`+fixtures.ContactTexterPhone+`","effective":"dont_track","provenance":"imported","state":"denied"`)
}
