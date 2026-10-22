// Package fixtures holds shared helpers for loading the committed YAML fixtures
// in fixtures/*.yml. The same fixtures back the test suite (internal/testhelper)
// and the dev seed (cmd/seed), so the loader options live here to stay in sync.
package fixtures

import (
	"strings"
	"text/template"
	"time"

	"github.com/go-crypt/crypt/algorithm/argon2"
	"golang.org/x/crypto/bcrypt"

	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// timeLayout is the Postgres-friendly timestamp layout the template date helpers
// emit. go-testfixtures hands these strings straight to the driver, and Postgres reads
// a zoneless string in the session's TimeZone, which shifts every fixture time by the
// server's UTC offset (a UTC day boundary then lands in the wrong day). The explicit
// UTC offset makes the instant the same on every server.
const timeLayout = "2006-01-02 15:04:05-07"

// TemplateFuncs returns the funcs available inside fixture YAML when the loader
// is built with testfixtures.Template(). Two problems make templating necessary:
//
//   - Dates must be relative to "now". Analytics reads broadcast_recipients.sent_at
//     within a window relative to time.Now(); frozen dates render an empty chart.
//     {{daysAgo 2}} / {{daysAhead 3}} keep seeded data inside the window forever.
//   - Encrypted columns can't be static. integration.config_encrypted and
//     webhook_endpoint.secret_encrypted are sealed with the env's SPHERICON_ENCRYPTION_KEY,
//     which differs between test and dev — one static ciphertext can't decrypt
//     under both. {{encrypt "<json>"}} seals at load time with the current env's
//     cipher, so the blob is always valid for the database being loaded.
//
// All times are anchored to one instant, now, so one load is internally consistent. The
// caller names it instead of this package reading the clock: a loader that runs inside a
// testing/synctest bubble would read the bubble's clock, which starts in the year 2000, and
// date every fixture around it.
func TemplateFuncs(cipher *secrets.Cipher, now time.Time) template.FuncMap {
	base := now.UTC()
	at := func(d time.Duration) string { return base.Add(d).Format(timeLayout) }
	day := 24 * time.Hour
	return template.FuncMap{
		"now":        func() string { return base.Format(timeLayout) },
		"daysAgo":    func(n int) string { return at(-time.Duration(n) * day) },
		"daysAhead":  func(n int) string { return at(time.Duration(n) * day) },
		"hoursAgo":   func(n int) string { return at(-time.Duration(n) * time.Hour) },
		"hoursAhead": func(n int) string { return at(time.Duration(n) * time.Hour) },
		// seq repeats a row: {{range $i := seq 100 199}} runs for 100 through 199, both included.
		// The loop renders inside one file, so one Load inserts it with the rest (a second Load of
		// the same table would wipe the first). The generator names a loop body's rows once.
		"seq": func(from, to int) []int {
			out := make([]int, 0, max(to-from+1, 0))
			for i := from; i <= to; i++ {
				out = append(out, i)
			}
			return out
		},
		// repeat builds a long value without writing it out: {{repeat "x" 33000}}.
		"repeat": strings.Repeat,
		// monthAgo is a moment inside a calendar month (UTC) some months back: {{monthAgo 6 3 10}} is the
		// 3rd, 10:00 of the month six months ago; a fourth number adds minutes, which may be negative
		// ({{monthAgo 6 1 0 -1}} is the last minute of the month before). Usage is measured per calendar month, so a scenario
		// that spans a month boundary needs rows pinned to a month, not to a number of days.
		"monthAgo": func(months, dayOfMonth, hour, minute int) string {
			start := time.Date(base.Year(), base.Month()-time.Month(months), 1, 0, 0, 0, 0, time.UTC)
			return start.AddDate(0, 0, dayOfMonth-1).Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute).Format(timeLayout)
		},
		"encrypt": func(s string) (string, error) { return cipher.Encrypt([]byte(s)) },
		// Invitation tokens are stored only as a SHA-256 hash; fixtures express the
		// raw token and hash it at load time so tests can present the raw value.
		"inviteHash": accounts.HashInviteToken,
		// Recovery codes are stored only as a hash (ADR 0020); the fixture states the
		// code and the generator lifts it into the catalog (fixtures.<Name>Code).
		"recoveryCodeHash": credentials.HashRecoveryCode,
		// Anchor credentials: the fixture states the plaintext and the hash is derived
		// at load time, so hash and secret cannot drift apart. The fixture generator
		// lifts the plaintext literal into the catalog (e.g. fixtures.OwnerJohnPassword).
		// Both hash at minimum cost: the production verify code reads the cost back
		// from the stored hash, so verification still works and loading stays fast.
		"argonHash":  hashPasswordMinCost,
		"bcryptHash": hashTokenSecretMinCost,
		// A Service credential's secret is stored as the SHA-256 the Staff member module
		// computes when it issues one (credentials.HashServiceSecret).
		"serviceSecretHash": credentials.HashServiceSecret,
		// A fixture Passkey states its credential id (base64url) and its private key (base64
		// PKCS8); the columns hold the raw id and the derived COSE public key.
		"passkeyCredentialID": passkeyCredentialID,
		"passkeyPublicKey":    passkeyPublicKey,
	}
}

// hashPasswordMinCost is a PHC argon2id hash at the minimum legal parameters
// (credentials.VerifyPassword decodes the parameters from the hash itself).
func hashPasswordMinCost(password string) (string, error) {
	hasher, err := argon2.New(
		argon2.WithVariantID(),
		argon2.WithT(argon2.IterationsMin),
		argon2.WithM(argon2.MemoryMin),
		argon2.WithP(argon2.ParallelismMin),
	)
	if err != nil {
		return "", err
	}
	digest, err := hasher.Hash(password)
	if err != nil {
		return "", err
	}
	return digest.Encode(), nil
}

// hashTokenSecretMinCost is a bcrypt hash at bcrypt.MinCost
// (credentials.VerifyTokenSecret reads the cost from the hash).
func hashTokenSecretMinCost(secret string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.MinCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
