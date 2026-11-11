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
	"errors"
	"fmt"
	"strings"

	"github.com/tink-crypto/tink-go/v2/aead"
	"github.com/tink-crypto/tink-go/v2/core/cryptofmt"
	"github.com/tink-crypto/tink-go/v2/insecurecleartextkeyset"
	"github.com/tink-crypto/tink-go/v2/keyset"
	tinkpb "github.com/tink-crypto/tink-go/v2/proto/tink_go_proto"
	"github.com/tink-crypto/tink-go/v2/tink"
)

// ErrCiphertext is returned when a stored value cannot be decrypted.
var ErrCiphertext = errors.New("invalid or corrupt ciphertext")

// ErrNoKeyset means the keyset is empty (SPHERICON_ENCRYPTION_KEY is not set): there is no key to
// build a Cipher from, to rotate or to retire.
var ErrNoKeyset = errors.New("SPHERICON_ENCRYPTION_KEY is not set: there is no encryption keyset")

// Cipher encrypts and decrypts secret values with a Tink AEAD primitive.
type Cipher struct {
	aead tink.AEAD
	// primaryPrefix is the output prefix of the primary key, which starts every value it seals.
	primaryPrefix string
}

// NewCipher builds a Cipher from a base64-encoded Tink keyset. It fails fast
// (caller wires this at boot) when the keyset is missing or malformed, or when its primary key does
// not write the TINK output prefix: only that prefix names the key a value was sealed under, which
// Current reads (a RAW key's empty prefix would match every value).
func NewCipher(keysetBase64 string) (*Cipher, error) {
	kh, err := readKeyset(keysetBase64)
	if err != nil {
		return nil, err
	}
	prefix, err := primaryPrefix(kh)
	if err != nil {
		return nil, err
	}
	a, err := aead.New(kh)
	if err != nil {
		return nil, fmt.Errorf("build AEAD: %w", err)
	}
	return &Cipher{aead: a, primaryPrefix: prefix}, nil
}

// primaryPrefix is the output prefix of the keyset's primary key, which must be TINK.
func primaryPrefix(kh *keyset.Handle) (string, error) {
	ks := insecurecleartextkeyset.KeysetMaterial(kh)
	for _, k := range ks.GetKey() {
		if k.GetKeyId() != ks.GetPrimaryKeyId() {
			continue
		}
		if k.GetOutputPrefixType() != tinkpb.OutputPrefixType_TINK {
			return "", fmt.Errorf("encryption keyset: primary key %d has the %s output prefix, not the TINK output prefix",
				k.GetKeyId(), k.GetOutputPrefixType())
		}
		return cryptofmt.OutputPrefix(k)
	}
	return "", errors.New("encryption keyset: no primary key")
}

func readKeyset(keysetBase64 string) (*keyset.Handle, error) {
	keysetBase64 = strings.TrimSpace(keysetBase64)
	if keysetBase64 == "" {
		return nil, ErrNoKeyset
	}
	raw, err := base64.StdEncoding.DecodeString(keysetBase64)
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

// Current reports whether a stored value is sealed under the primary key: whether it starts with
// the primary key's output prefix, which names the key that sealed it.
func (c *Cipher) Current(value string) (bool, error) {
	ct, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(ct) < cryptofmt.NonRawPrefixSize {
		return false, fmt.Errorf("%w: no key id", ErrCiphertext)
	}
	return strings.HasPrefix(string(ct), c.primaryPrefix), nil
}

// Rewrap returns value sealed under the primary key, and whether it had to change: a value that
// is already current comes back as it is, so re-encrypting is idempotent.
func (c *Cipher) Rewrap(value string) (rewrapped string, changed bool, err error) {
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
