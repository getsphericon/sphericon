// Package secrets provides authenticated encryption for credentials stored at
// rest (e.g. workspace provider configs), backed by Google Tink. Tink ciphertext
// carries the key id, so key rotation is native (a keyset can hold multiple keys
// with one primary for encryption). The keyset is loaded from config as a
// base64-encoded cleartext keyset today; swapping in a KMS-wrapped keyset later
// is a load-time change that leaves Encrypt/Decrypt callers untouched.
package secrets

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/tink-crypto/tink-go/v2/aead"
	"github.com/tink-crypto/tink-go/v2/insecurecleartextkeyset"
	"github.com/tink-crypto/tink-go/v2/keyset"
	"github.com/tink-crypto/tink-go/v2/tink"
)

// ErrCiphertext is returned when a stored value cannot be decrypted.
var ErrCiphertext = errors.New("invalid or corrupt ciphertext")

// Cipher encrypts and decrypts secret values with a Tink AEAD primitive.
type Cipher struct {
	aead      tink.AEAD
	primaryID uint32
}

// NewCipher builds a Cipher from a base64-encoded Tink keyset. It fails fast
// (caller wires this at boot) when the keyset is missing or malformed.
func NewCipher(keysetBase64 string) (*Cipher, error) {
	if strings.TrimSpace(keysetBase64) == "" {
		return nil, errors.New("encryption keyset is required")
	}
	kh, err := readKeyset(keysetBase64)
	if err != nil {
		return nil, err
	}
	a, err := aead.New(kh)
	if err != nil {
		return nil, fmt.Errorf("build AEAD: %w", err)
	}
	return &Cipher{aead: a, primaryID: kh.KeysetInfo().GetPrimaryKeyId()}, nil
}

func readKeyset(keysetBase64 string) (*keyset.Handle, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(keysetBase64))
	if err != nil {
		return nil, fmt.Errorf("decode encryption keyset: %w", err)
	}
	kh, err := insecurecleartextkeyset.Read(keyset.NewBinaryReader(bytes.NewReader(raw)))
	if err != nil {
		return nil, fmt.Errorf("read encryption keyset: %w", err)
	}
	return kh, nil
}

func writeKeyset(kh *keyset.Handle) (string, error) {
	buf := new(bytes.Buffer)
	if err := insecurecleartextkeyset.Write(kh, keyset.NewBinaryWriter(buf)); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// Rotate adds a new AES-256-GCM key to the keyset, makes it the primary and returns the new
// keyset base64-encoded. Every earlier key stays, so ciphertext written under it still decrypts.
func Rotate(keysetBase64 string) (string, error) {
	kh, err := readKeyset(keysetBase64)
	if err != nil {
		return "", err
	}
	m := keyset.NewManagerFromHandle(kh)
	id, err := m.Add(aead.AES256GCMKeyTemplate())
	if err != nil {
		return "", fmt.Errorf("add key: %w", err)
	}
	if err := m.SetPrimary(id); err != nil {
		return "", fmt.Errorf("set primary key: %w", err)
	}
	if kh, err = m.Handle(); err != nil {
		return "", err
	}
	return writeKeyset(kh)
}

// Retire drops every key but the primary from the keyset and returns it base64-encoded. Run it
// only once every stored value is under the primary key (see Cipher.Current): ciphertext written
// under a dropped key can no longer be decrypted.
func Retire(keysetBase64 string) (string, error) {
	kh, err := readKeyset(keysetBase64)
	if err != nil {
		return "", err
	}
	primary := kh.KeysetInfo().GetPrimaryKeyId()
	m := keyset.NewManagerFromHandle(kh)
	for _, k := range kh.KeysetInfo().GetKeyInfo() {
		if k.GetKeyId() == primary {
			continue
		}
		if err := m.Delete(k.GetKeyId()); err != nil {
			return "", fmt.Errorf("delete key %d: %w", k.GetKeyId(), err)
		}
	}
	if kh, err = m.Handle(); err != nil {
		return "", err
	}
	return writeKeyset(kh)
}

// Current reports whether a stored value is sealed under the primary key. Tink ciphertext starts
// with the byte 1 and the 4-byte big-endian id of the key that sealed it.
func (c *Cipher) Current(value string) (bool, error) {
	ct, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(ct) < 5 {
		return false, fmt.Errorf("%w: no key id", ErrCiphertext)
	}
	return ct[0] == 1 && binary.BigEndian.Uint32(ct[1:5]) == c.primaryID, nil
}

// Rewrap returns value sealed under the primary key, and whether it had to change: a value that
// is already current comes back as it is, so re-encrypting is idempotent.
func (c *Cipher) Rewrap(value string) (string, bool, error) {
	current, err := c.Current(value)
	if err != nil || current {
		return value, false, err
	}
	pt, err := c.Decrypt(value)
	if err != nil {
		return "", false, err
	}
	sealed, err := c.Encrypt(pt)
	return sealed, true, err
}

// Encrypt seals plaintext and returns a base64 string safe to store in a text column.
func (c *Cipher) Encrypt(plaintext []byte) (string, error) {
	ct, err := c.aead.Encrypt(plaintext, nil)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt reverses Encrypt.
func (c *Cipher) Decrypt(value string) ([]byte, error) {
	ct, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCiphertext, err)
	}
	pt, err := c.aead.Decrypt(ct, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCiphertext, err)
	}
	return pt, nil
}

// GenerateKeysetBase64 mints a fresh AES-256-GCM keyset and returns it
// base64-encoded — for seeding env config and tests.
func GenerateKeysetBase64() (string, error) {
	kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
	if err != nil {
		return "", err
	}
	buf := new(bytes.Buffer)
	if err := insecurecleartextkeyset.Write(kh, keyset.NewBinaryWriter(buf)); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
