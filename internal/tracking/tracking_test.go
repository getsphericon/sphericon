package tracking_test

import (
	"strings"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenRoundTrip(t *testing.T) {
	tr := tracking.New("secret", "https://app.test")
	tok, err := tr.Token(42)
	require.NoError(t, err)

	rid, err := tr.Decode(tok)
	require.NoError(t, err)
	assert.Equal(t, int64(42), rid)
}

func TestDecodeRejectsWrongSecret(t *testing.T) {
	tok, err := tracking.New("secret", "https://app.test").Token(1)
	require.NoError(t, err)

	_, err = tracking.New("other-secret", "https://app.test").Decode(tok)
	assert.Error(t, err)
}

// unsubToken↔DecodeUnsub round-trips every field, including a large int64 id that
// would lose precision if stored as a JSON number (float64) instead of a string.
func TestUnsubTokenRoundTrip(t *testing.T) {
	tr := tracking.New("secret", "https://app.test")
	target := tracking.UnsubTarget{
		Source:      "automation:7",
		Destination: "person@example.com",
		WorkspaceID: fixtures.AcmeID,
		ContactID:   9007199254740993, // 2^53 + 1: not representable as float64
		BroadcastID: 0,
	}
	urlStr, err := tr.UnsubscribeURL(target)
	require.NoError(t, err)
	assert.Contains(t, urlStr, "https://app.test/e/u/")

	token := strings.TrimPrefix(urlStr, "https://app.test/e/u/")
	got, err := tr.DecodeUnsub(token)
	require.NoError(t, err)
	assert.Equal(t, target, got)
}

// confirmToken↔DecodeConfirm round-trips every field, including a large int64 id
// that would lose precision as a JSON number (float64) instead of a string.
func TestConfirmTokenRoundTrip(t *testing.T) {
	tr := tracking.New("secret", "https://app.test")
	target := tracking.ConfirmTarget{
		Destination: "person@example.com",
		WorkspaceID: fixtures.AcmeID,
		ContactID:   9007199254740993, // 2^53 + 1: not representable as float64
	}
	urlStr, err := tr.ConfirmURL(target)
	require.NoError(t, err)
	assert.Contains(t, urlStr, "https://app.test/e/confirm/")

	token := strings.TrimPrefix(urlStr, "https://app.test/e/confirm/")
	got, err := tr.DecodeConfirm(token)
	require.NoError(t, err)
	assert.Equal(t, target, got)
}

// A confirmation link expires (ADR 0013): DecodeConfirm rejects a token whose exp
// has passed, so a stale link cannot confirm.
func TestDecodeConfirmRejectsExpired(t *testing.T) {
	tr := tracking.New("secret", "https://app.test")
	// Hand-mint a token with the confirm claim shape but an exp in the past.
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"dest": "person@example.com",
		"ws":   "1",
		"cid":  "2",
		"exp":  time.Now().Add(-time.Hour).Unix(),
	})
	signed, err := tok.SignedString([]byte("secret"))
	require.NoError(t, err)

	_, err = tr.DecodeConfirm(signed)
	require.Error(t, err, "expired confirmation token is rejected")
	assert.True(t, tracking.IsExpired(err), "and recognised as expired")

	_, err = tr.DecodeConfirm("garbage")
	require.Error(t, err)
	assert.False(t, tracking.IsExpired(err), "a malformed token is invalid, not expired")
}

func TestRewriteWrapsLinksAndAddsPixelAndFooter(t *testing.T) {
	tr := tracking.New("secret", "https://app.test")
	body := `<p>Hello <a href="https://dest.test/path?x=1">click</a> or <a href="mailto:a@b.test">mail</a></p>`

	out, err := tr.Rewrite(body, 7, tracking.UnsubTarget{
		Source: "broadcasts", Destination: "x@y.test", WorkspaceID: fixtures.AcmeID, ContactID: 2, BroadcastID: 3,
	}, "Acme Inc, 123 Main St, Springfield")
	require.NoError(t, err)

	// http link routed through the click tracker, with the destination signed into the token.
	_, rest, found := strings.Cut(out, "https://app.test/e/c/")
	require.True(t, found, out)
	clickToken, _, _ := strings.Cut(rest, `"`)
	assert.NotContains(t, clickToken, "?", "the destination is not a bare parameter")
	rid, dest, err := tr.DecodeClick(clickToken)
	require.NoError(t, err)
	assert.Equal(t, int64(7), rid)
	assert.Equal(t, "https://dest.test/path?x=1", dest)
	// mailto links are left untouched.
	assert.Contains(t, out, `href="mailto:a@b.test"`)
	// open pixel + unsubscribe footer appended.
	assert.Contains(t, out, "https://app.test/e/o/")
	assert.Contains(t, out, "https://app.test/e/u/")
	assert.Contains(t, strings.ToLower(out), "unsubscribe")
	// CAN-SPAM: the workspace's physical postal address rides in the footer.
	assert.Contains(t, out, "Acme Inc, 123 Main St, Springfield")
}

func TestDecodeClickRefusesTokensWithoutASignedDestination(t *testing.T) {
	tr := tracking.New("secret", "https://app.test")

	recipient, err := tr.Token(7)
	require.NoError(t, err)
	_, _, err = tr.DecodeClick(recipient)
	require.Error(t, err, "the open-pixel token carries no destination")

	confirmURL, err := tr.ConfirmURL(tracking.ConfirmTarget{Destination: "https://x.test", WorkspaceID: fixtures.AcmeID, ContactID: 2})
	require.NoError(t, err)
	_, confirm, _ := strings.Cut(confirmURL, "/e/confirm/")
	_, _, err = tr.DecodeClick(confirm)
	require.Error(t, err, "a confirm token has a dest claim but no recipient")

	link, err := tr.ClickURL(7, "javascript:alert(1)")
	require.NoError(t, err)
	_, js, _ := strings.Cut(link, "/e/c/")
	_, _, err = tr.DecodeClick(js)
	require.Error(t, err, "only http(s) destinations redirect")

	link, err = tr.ClickURL(7, "https://dest.test/")
	require.NoError(t, err)
	_, good, _ := strings.Cut(link, "/e/c/")
	_, _, err = tracking.New("other", "https://app.test").DecodeClick(good)
	require.Error(t, err, "a token signed with another secret is refused")
}

func TestUnsubscribeFooterPostalAddress(t *testing.T) {
	tr := tracking.New("secret", "https://app.test")
	target := tracking.UnsubTarget{Source: "broadcasts", Destination: "x@y.test", WorkspaceID: fixtures.AcmeID}

	// Empty address: the footer renders the unsubscribe line only (non-gating).
	bare, err := tr.UnsubscribeFooter(target, "")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(bare), "unsubscribe")

	// A multi-line address is HTML-escaped and its newlines become <br>.
	withAddr, err := tr.UnsubscribeFooter(target, "Acme <Inc>\n123 Main St")
	require.NoError(t, err)
	assert.Contains(t, withAddr, "Acme &lt;Inc&gt;<br>123 Main St")
}

// A JWT secret rotation: a link mailed before it still verifies while the old secret is the
// secondary one, and new links are signed with the primary.
func TestDecodeAcceptsTheSecondarySecretButSignsWithThePrimary(t *testing.T) {
	old := tracking.New("old-secret", "https://app.test")
	mailed, err := old.Token(5)
	require.NoError(t, err)
	unsub, err := old.UnsubscribeURL(tracking.UnsubTarget{Source: "broadcasts", Destination: "a@b.test", WorkspaceID: 2})
	require.NoError(t, err)

	rotated := tracking.New("new-secret", "https://app.test", "old-secret")
	rid, err := rotated.Decode(mailed)
	require.NoError(t, err)
	assert.EqualValues(t, 5, rid)
	_, err = rotated.DecodeUnsub(strings.TrimPrefix(unsub, "https://app.test/e/u/"))
	require.NoError(t, err)

	fresh, err := rotated.Token(6)
	require.NoError(t, err)
	_, err = tracking.New("new-secret", "https://app.test").Decode(fresh)
	require.NoError(t, err, "a new link verifies without the secondary secret")
	_, err = old.Decode(fresh)
	require.Error(t, err)

	_, err = tracking.New("new-secret", "https://app.test").Decode(mailed)
	assert.Error(t, err, "once the secondary secret is removed, the old link no longer verifies")
}
