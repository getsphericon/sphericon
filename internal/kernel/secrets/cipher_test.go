package secrets_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tink-crypto/tink-go/v2/aead"
	"github.com/tink-crypto/tink-go/v2/insecurecleartextkeyset"
	"github.com/tink-crypto/tink-go/v2/keyset"

	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

func testCipher(t *testing.T) *secrets.Cipher {
	t.Helper()
	ks, err := secrets.GenerateKeysetBase64()
	require.NoError(t, err)
	c, err := secrets.NewCipher(ks)
	require.NoError(t, err)
	return c
}

func TestCipherRoundTrip(t *testing.T) {
	c := testCipher(t)

	plaintext := []byte(`{"host":"smtp.example.com","password":"s3cret"}`)
	enc, err := c.Encrypt(plaintext)
	require.NoError(t, err)
	assert.NotContains(t, enc, "s3cret", "secret is not stored in cleartext")

	dec, err := c.Decrypt(enc)
	require.NoError(t, err)
	assert.Equal(t, plaintext, dec)
}

func TestNewCipherRejectsBadKeyset(t *testing.T) {
	_, err := secrets.NewCipher("")
	require.Error(t, err)

	_, err = secrets.NewCipher("not-base64!!!")
	require.Error(t, err)

	_, err = secrets.NewCipher("aGVsbG8td29ybGQ=") // valid base64, not a keyset
	require.Error(t, err)
}

func TestDecryptRejectsTamperedOrForeign(t *testing.T) {
	c := testCipher(t)

	_, err := c.Decrypt("not-base64!!!")
	require.ErrorIs(t, err, secrets.ErrCiphertext)

	enc, err := c.Encrypt([]byte("payload"))
	require.NoError(t, err)
	_, err = c.Decrypt(enc + "tampered")
	require.ErrorIs(t, err, secrets.ErrCiphertext)

	// Ciphertext from a different keyset must not decrypt.
	other := testCipher(t)
	foreign, err := other.Encrypt([]byte("payload"))
	require.NoError(t, err)
	_, err = c.Decrypt(foreign)
	assert.ErrorIs(t, err, secrets.ErrCiphertext)
}

// A RAW primary key writes no key id into its ciphertext: its empty prefix would match every stored
// value, so Current could never tell an old value from a current one.
func TestNewCipherRefusesAPrimaryKeyWithoutTheTinkPrefix(t *testing.T) {
	kh, err := keyset.NewHandle(aead.AES256GCMNoPrefixKeyTemplate())
	require.NoError(t, err)
	buf := new(bytes.Buffer)
	require.NoError(t, insecurecleartextkeyset.Write(kh, keyset.NewBinaryWriter(buf)))

	_, err = secrets.NewCipher(base64.StdEncoding.EncodeToString(buf.Bytes()))

	assert.ErrorContains(t, err, "TINK output prefix")
}
