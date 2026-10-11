// Package otpcode is the TOTP arithmetic shared by every second step (a User's Second
// factor, ADR 0020, and an Operator's TOTP, ADR 0026): new keys with their QR code, and
// matching a submitted code against a secret at a clock. It holds no state: each caller
// stores the last accepted time step itself and makes the conditional write that makes
// a code single-use.
package otpcode

import (
	"bytes"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"image/png"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	period = 30 // seconds per TOTP time step
	// skew is how many steps either side of now a code is accepted for (clock drift).
	skew   = 1
	qrSize = 200
)

var validateOpts = totp.ValidateOpts{Period: period, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}

// Key is a freshly generated TOTP secret, shown so it can be added to an app.
type Key struct {
	Secret string // base32 key
	URI    string // otpauth:// URI
	QRCode string // PNG data URI of the URI
}

// New generates a key for account under issuer.
func New(issuer, account string) (*Key, error) {
	return generate(issuer, account, nil)
}

// FromSecret rebuilds the Key of an existing base32 secret, so a pending enrolment can
// be shown again unchanged.
func FromSecret(issuer, account, secret string) (*Key, error) {
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return nil, err
	}
	return generate(issuer, account, raw)
}

// generate builds a Key over secret, or over a new random one when secret is nil.
func generate(issuer, account string, secret []byte) (*Key, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer: issuer, AccountName: account, Secret: secret,
		Period: period, Digits: validateOpts.Digits, Algorithm: validateOpts.Algorithm,
	})
	if err != nil {
		return nil, err
	}
	img, err := key.Image(qrSize, qrSize)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return &Key{
		Secret: key.Secret(),
		URI:    key.URL(),
		QRCode: "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()),
	}, nil
}

// Match reports the time step code is the secret's code for, among the steps within the
// skew of now and later than lastStep (so a code of a step already accepted never
// matches). ok is false when no such step matches.
func Match(secret, code string, now time.Time, lastStep int64) (step int64, ok bool, err error) {
	code = strings.TrimSpace(code)
	current := now.Unix() / period
	for step := current - skew; step <= current+skew; step++ {
		if step <= lastStep {
			continue
		}
		want, err := totp.GenerateCodeCustom(secret, time.Unix(step*period, 0), validateOpts)
		if err != nil {
			return 0, false, err
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true, nil
		}
	}
	return 0, false, nil
}

// IsCode reports whether s has the shape of a TOTP code (six digits).
func IsCode(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != int(validateOpts.Digits) {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
