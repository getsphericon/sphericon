package licensekey_test

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/licensekey"
)

// The key is read from the environment at boot: whatever it holds, Parse answers with a License
// or an error, never both and never a panic, and a key nobody signed never licenses anything.
func FuzzParseOfAnUnsignedKeyNeverLicenses(f *testing.F) {
	f.Add("")
	f.Add("a.b.c")
	f.Add("eyJhbGciOiJFZERTQSIsInR5cCI6IkpXVCJ9.e30.")
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		f.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	f.Fuzz(func(t *testing.T, key string) {
		license, err := licensekey.Parse(key, pub, now)

		require.NotEqualf(t, license == nil, err == nil, "Parse(%q) returned license %v and error %v", key, license, err)
		accepted := key != "" && err == nil
		assert.Falsef(t, accepted, "Parse accepted %q, which nobody signed", key)
	})
}
