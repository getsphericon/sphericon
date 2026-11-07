package testhelper

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// Cipher is the cipher of the test configuration's encryption key: the one the fixtures are sealed
// with and Setup's server seals with. A test with a Setup env takes env.Cipher; Cipher is for a test
// that builds its modules without one (a race over RealDB, a river runtime, the fixture loader).
func Cipher(tb testing.TB) *secrets.Cipher {
	tb.Helper()
	cfg, err := config.Load("test")
	require.NoError(tb, err)
	c, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(tb, err)
	return c
}
