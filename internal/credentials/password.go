package credentials

import (
	"fmt"

	"github.com/go-crypt/crypt"
	"github.com/go-crypt/crypt/algorithm/argon2"
)

// User passwords are hashed with argon2id via go-crypt/crypt. The algorithm
// parameters are encoded into the hash string itself (PHC format), so there is
// no cost constant to keep in sync and Verify reads them back from the stored
// hash. API token secrets are deliberately NOT hashed this way — they are
// verified on every request, where argon2id's memory cost is prohibitive; see
// HashTokenSecret/VerifyTokenSecret.
var (
	passwordHasher  = newPasswordHasher()
	passwordDecoder = newPasswordDecoder()
)

func newPasswordHasher() *argon2.Hasher {
	h, err := argon2.New(argon2.WithProfileRFC9106LowMemory())
	if err != nil {
		panic(fmt.Sprintf("service: argon2 hasher: %v", err))
	}
	return h
}

func newPasswordDecoder() *crypt.Decoder {
	d := crypt.NewDecoder()
	if err := argon2.RegisterDecoderArgon2id(d); err != nil {
		panic(fmt.Sprintf("service: argon2 decoder: %v", err))
	}
	return d
}

// HashPassword returns a PHC-encoded argon2id hash of the given password.
func HashPassword(password string) (string, error) {
	digest, err := passwordHasher.Hash(password)
	if err != nil {
		return "", err
	}
	return digest.Encode(), nil
}

// VerifyPassword reports whether password matches the PHC-encoded hash.
func VerifyPassword(encodedHash, password string) bool {
	digest, err := passwordDecoder.Decode(encodedHash)
	if err != nil {
		return false
	}
	return digest.Match(password)
}
