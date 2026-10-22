package tracking_test

import (
	"maps"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/tracking"
)

const forgeSecret = "secret"

// signed returns a token validly signed with the tracker's secret but carrying
// arbitrary claims, so Decode* must reject it on content, not signature.
func signed(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(forgeSecret))
	require.NoError(t, err)
	return tok
}

func newTracker() *tracking.Tracker { return tracking.New(forgeSecret, "https://app.test") }

func TestDecodeRejectsMalformedClaims(t *testing.T) {
	tr := newTracker()
	for name, claims := range map[string]jwt.MapClaims{
		"missing mid": {},
		"numeric mid": {"mid": 5},
		"non-int mid": {"mid": "five"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := tr.Decode(signed(t, claims))
			assert.Error(t, err)
		})
	}
	_, err := tr.Decode("garbage")
	assert.Error(t, err)
}

func TestDecodeUnsubRejectsMalformedClaims(t *testing.T) {
	tr := newTracker()
	ok := jwt.MapClaims{"src": "broadcasts", "dest": "a@b.com", "ws": "1", "cid": "2", "bid": "3"}
	with := func(k string, v any) jwt.MapClaims {
		c := jwt.MapClaims{}
		maps.Copy(c, ok)
		if v == nil {
			delete(c, k)
		} else {
			c[k] = v
		}
		return c
	}

	got, err := tr.DecodeUnsub(signed(t, ok))
	require.NoError(t, err)
	assert.Equal(t, tracking.UnsubTarget{Source: "broadcasts", Destination: "a@b.com", WorkspaceID: 1, ContactID: 2, BroadcastID: 3}, got)

	for name, claims := range map[string]jwt.MapClaims{
		"missing src": with("src", nil),
		"empty src":   with("src", ""),
		"missing ws":  with("ws", nil),
		"bad ws":      with("ws", "x"),
		"bad cid":     with("cid", "x"),
		"bad bid":     with("bid", "x"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := tr.DecodeUnsub(signed(t, claims))
			assert.Error(t, err)
		})
	}
	// A contact or broadcast id is optional: absent means "none" (0).
	none, err := tr.DecodeUnsub(signed(t, with("bid", nil)))
	require.NoError(t, err)
	assert.Zero(t, none.BroadcastID)
	_, err = tr.DecodeUnsub("garbage")
	assert.Error(t, err)
}

func TestDecodeConfirmRejectsMalformedClaims(t *testing.T) {
	tr := newTracker()
	exp := time.Now().Add(time.Hour).Unix()
	with := func(k string, v any) jwt.MapClaims {
		c := jwt.MapClaims{"dest": "a@b.com", "ws": "1", "cid": "2", "exp": exp}
		if v == nil {
			delete(c, k)
		} else {
			c[k] = v
		}
		return c
	}

	got, err := tr.DecodeConfirm(signed(t, with("dest", "a@b.com")))
	require.NoError(t, err)
	assert.Equal(t, tracking.ConfirmTarget{Destination: "a@b.com", WorkspaceID: 1, ContactID: 2}, got)

	for name, claims := range map[string]jwt.MapClaims{
		"missing dest": with("dest", nil),
		"missing ws":   with("ws", nil),
		"bad ws":       with("ws", "x"),
		"bad cid":      with("cid", "x"),
		"missing exp":  with("exp", nil),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := tr.DecodeConfirm(signed(t, claims))
			require.Error(t, err)
			assert.False(t, tracking.IsExpired(err), "malformed is not expired")
		})
	}
	_, err = tr.DecodeConfirm("garbage")
	require.Error(t, err)
	assert.False(t, tracking.IsExpired(err))
}

func TestURLsCarryDecodableTokens(t *testing.T) {
	tr := tracking.New(forgeSecret, "https://app.test/")

	u, err := tr.UnsubscribeURL(tracking.UnsubTarget{Source: "broadcasts", Destination: "a@b.com", WorkspaceID: 1})
	require.NoError(t, err)
	require.Contains(t, u, "https://app.test/e/u/")
	target, err := tr.DecodeUnsub(u[len("https://app.test/e/u/"):])
	require.NoError(t, err)
	assert.Equal(t, "a@b.com", target.Destination)

	c, err := tr.ConfirmURL(tracking.ConfirmTarget{Destination: "a@b.com", WorkspaceID: 1, ContactID: 2})
	require.NoError(t, err)
	require.Contains(t, c, "https://app.test/e/confirm/")
	ct, err := tr.DecodeConfirm(c[len("https://app.test/e/confirm/"):])
	require.NoError(t, err)
	assert.Equal(t, int64(2), ct.ContactID)
}
