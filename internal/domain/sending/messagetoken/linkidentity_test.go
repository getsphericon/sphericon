package messagetoken_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/kernel/publicid"
)

// recipient is the Contact the tests' Link identity tokens name.
var recipient = messagetoken.LinkIdentity{WorkspaceID: 1, ContactID: 9007199254740993}

func TestALinkIdentityTokenNamesItsContactForSevenDaysFromTheSend(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	tr := messagetoken.New(forgeSecret, "https://app.test", func() time.Time { return now })

	fresh, err := tr.LinkIdentityToken(recipient, now.Add(-6*24*time.Hour))
	require.NoError(t, err)
	got, err := tr.DecodeLinkIdentity(fresh)
	require.NoError(t, err)
	assert.Equal(t, recipient, got)

	stale, err := tr.LinkIdentityToken(recipient, now.Add(-8*24*time.Hour))
	require.NoError(t, err)
	_, err = tr.DecodeLinkIdentity(stale)
	assert.Error(t, err)
}

// An unsubscribe or a confirmation link names a Contact too, and the unsubscribe one never
// expires: neither is read as a Link identity token (ADR 0160).
func TestNoOtherLinkTokenOfTheContactIsALinkIdentityToken(t *testing.T) {
	tr := newTracker()
	unsub, err := tr.UnsubscribeURL(linktarget.Unsubscribe{
		Channel: channels.Email, Source: "broadcasts", Destination: "a@b.com",
		WorkspaceID: recipient.WorkspaceID, ContactID: recipient.ContactID,
	})
	require.NoError(t, err)
	confirm, err := tr.ConfirmURL(linktarget.Confirmation{Destination: "a@b.com", WorkspaceID: recipient.WorkspaceID, ContactID: recipient.ContactID})
	require.NoError(t, err)

	for name, token := range map[string]string{
		"unsubscribe":  unsub[len("https://app.test/e/u/"):],
		"confirmation": confirm[len("https://app.test/e/confirm/"):],
	} {
		t.Run(name, func(t *testing.T) {
			_, err := tr.DecodeLinkIdentity(token)
			assert.Error(t, err)
		})
	}
}

// linkIdentityClaims are the claims of a valid Link identity token with k set to v, or left out
// when v is nil.
func linkIdentityClaims(k string, v any) jwt.MapClaims {
	c := jwt.MapClaims{
		"aud": "link-identity", "ws": publicid.Encode(1), "cid": publicid.Encode(2),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	if v == nil {
		delete(c, k)
		return c
	}
	c[k] = v
	return c
}

func TestDecodeLinkIdentityRejectsMalformedClaims(t *testing.T) {
	tr := newTracker()
	with := linkIdentityClaims

	got, err := tr.DecodeLinkIdentity(signed(t, with("aud", "link-identity")))
	require.NoError(t, err)
	assert.Equal(t, messagetoken.LinkIdentity{WorkspaceID: 1, ContactID: 2}, got)

	for name, claims := range map[string]jwt.MapClaims{
		"missing aud": with("aud", nil),
		"other aud":   with("aud", "unsubscribe"),
		"missing exp": with("exp", nil),
		"missing ws":  with("ws", nil),
		"missing cid": with("cid", nil),
		"bad cid":     with("cid", "x"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := tr.DecodeLinkIdentity(signed(t, claims))
			assert.Error(t, err)
		})
	}
	_, err = messagetoken.New("other-secret", "https://app.test", time.Now).DecodeLinkIdentity(signed(t, with("aud", "link-identity")))
	assert.Error(t, err)
}
