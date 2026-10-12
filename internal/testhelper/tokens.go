package testhelper

import (
	"strconv"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// ExpiredConfirmToken is a confirmation token for target, signed like the
// Tracker's but already past its exp: the state a link reaches after its TTL.
// Mint live tokens with env.Tracker; this exists only for the expiry path.
func (e *TestEnv) ExpiredConfirmToken(t *testing.T, target tracking.ConfirmTarget) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"dest": target.Destination,
		"ws":   strconv.FormatInt(target.WorkspaceID, 10),
		"cid":  strconv.FormatInt(target.ContactID, 10),
		"exp":  time.Now().Add(-time.Hour).Unix(),
	}).SignedString([]byte(e.jwtSecret))
	require.NoError(t, err)
	return signed
}
