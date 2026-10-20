package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCipher(t *testing.T) *Cipher {
	t.Helper()
	ks, err := GenerateKeysetBase64()
	require.NoError(t, err)
	c, err := NewCipher(ks)
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
	_, err := NewCipher("")
	require.Error(t, err)

	_, err = NewCipher("not-base64!!!")
	require.Error(t, err)

	_, err = NewCipher("aGVsbG8td29ybGQ=") // valid base64, not a keyset
	require.Error(t, err)
}

func TestDecryptRejectsTamperedOrForeign(t *testing.T) {
	c := testCipher(t)

	_, err := c.Decrypt("not-base64!!!")
	require.ErrorIs(t, err, ErrCiphertext)

	enc, err := c.Encrypt([]byte("payload"))
	require.NoError(t, err)
	_, err = c.Decrypt(enc + "tampered")
	require.ErrorIs(t, err, ErrCiphertext)

	// Ciphertext from a different keyset must not decrypt.
	other := testCipher(t)
	foreign, err := other.Encrypt([]byte("payload"))
	require.NoError(t, err)
	_, err = c.Decrypt(foreign)
	assert.ErrorIs(t, err, ErrCiphertext)
}
