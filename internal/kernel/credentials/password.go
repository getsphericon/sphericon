package credentials

import (
	"fmt"

	"github.com/go-crypt/crypt"
	"github.com/go-crypt/crypt/algorithm/argon2"
)

// User passwords are hashed with argon2id via go-crypt/crypt. The algorithm
// parameters are encoded into the hash string itself (PHC format), so there is
// no cost constant to keep in sync and VerifyPassword reads them back from the stored
// hash. Generated secrets (API tokens, Service credentials) are deliberately NOT
// hashed this way — they are random and verified on every request, where a slow
// hash is a cost with no gain; see HashSecret/VerifySecret.
var passwordDecoder = newPasswordDecoder()

func newPasswordDecoder() *crypt.Decoder {
	d := crypt.NewDecoder()
	if err := argon2.RegisterDecoderArgon2id(d); err != nil {
		panic(fmt.Sprintf("service: argon2 decoder: %v", err))
	}
	return d
}

// Hasher hashes passwords into PHC-encoded argon2id strings. The server hashes with
// NewHasher; a hash is verified by VerifyPassword whatever parameters made it.
type Hasher struct {
	argon *argon2.Hasher
}

// NewHasher hashes at the RFC 9106 low-memory profile: the cost every stored User and Staff
// member password carries.
func NewHasher() *Hasher {
	return newHasher(argon2.WithProfileRFC9106LowMemory())
}

// NewMinimumCostHasher hashes at the lowest parameters argon2id allows, for the passwords of
// the committed fixtures and of a test run, where a password is known and only that its hash
// verifies matters: the real profile takes 64 MiB and tens of milliseconds a hash.
func NewMinimumCostHasher() *Hasher {
	return newHasher(
		argon2.WithVariantID(),
		argon2.WithT(argon2.IterationsMin),
		argon2.WithM(argon2.MemoryMin),
		argon2.WithP(argon2.ParallelismMin),
	)
}

func newHasher(opts ...argon2.Opt) *Hasher {
	h, err := argon2.New(opts...)
	if err != nil {
		panic(fmt.Sprintf("service: argon2 hasher: %v", err))
	}
	return &Hasher{argon: h}
}

// Hash returns a PHC-encoded argon2id hash of password.
func (h *Hasher) Hash(password string) (string, error) {
	digest, err := h.argon.Hash(password)
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
