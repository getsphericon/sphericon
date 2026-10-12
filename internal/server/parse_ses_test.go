package server

import (
	"testing"

	"github.com/getsphericon/sphericon/internal/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSESNotification(t *testing.T) {
	t.Run("permanent bounce", func(t *testing.T) {
		msg := `{"notificationType":"Bounce","bounce":{"bounceType":"Permanent","bouncedRecipients":[{"emailAddress":"a@example.com"},{"emailAddress":"b@example.com"}]}}`
		out, err := parseSESNotification(msg)
		require.NoError(t, err)
		require.Len(t, out, 2)
		assert.Equal(t, events.NameEmailBounced, out[0].Action)
		assert.Equal(t, events.BounceKindPermanent, out[0].Kind)
		assert.Equal(t, "a@example.com", out[0].Email)
		assert.Equal(t, "b@example.com", out[1].Email)
	})

	t.Run("transient bounce", func(t *testing.T) {
		msg := `{"notificationType":"Bounce","bounce":{"bounceType":"Transient","bouncedRecipients":[{"emailAddress":"soft@example.com"}]}}`
		out, err := parseSESNotification(msg)
		require.NoError(t, err)
		require.Len(t, out, 1)
		assert.Equal(t, events.BounceKindTransient, out[0].Kind)
	})

	t.Run("complaint", func(t *testing.T) {
		msg := `{"notificationType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":"spam@example.com"}]}}`
		out, err := parseSESNotification(msg)
		require.NoError(t, err)
		require.Len(t, out, 1)
		assert.Equal(t, events.NameEmailComplained, out[0].Action)
		assert.Empty(t, out[0].Kind)
		assert.Equal(t, "spam@example.com", out[0].Email)
	})

	t.Run("configuration-set eventType form", func(t *testing.T) {
		msg := `{"eventType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":"x@example.com"}]}}`
		out, err := parseSESNotification(msg)
		require.NoError(t, err)
		require.Len(t, out, 1)
		assert.Equal(t, events.NameEmailComplained, out[0].Action)
	})

	// ADR 0011: the failure Event carries the Sending domain, taken from the From
	// address SES echoes in mail.source (display name and case are normalized away).
	t.Run("sending domain from mail.source", func(t *testing.T) {
		msg := `{"notificationType":"Complaint","mail":{"source":"Acme <News@Mail.Acme.com>"},"complaint":{"complainedRecipients":[{"emailAddress":"spam@example.com"}]}}`
		out, err := parseSESNotification(msg)
		require.NoError(t, err)
		require.Len(t, out, 1)
		assert.Equal(t, "mail.acme.com", out[0].SendingDomain)
	})

	t.Run("delivery is ignored", func(t *testing.T) {
		out, err := parseSESNotification(`{"notificationType":"Delivery","delivery":{}}`)
		require.NoError(t, err)
		assert.Empty(t, out)
	})

	t.Run("malformed", func(t *testing.T) {
		_, err := parseSESNotification(`{not json`)
		assert.Error(t, err)
	})
}
