package secrets_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tink-crypto/tink-go/v2/insecurecleartextkeyset"
	"github.com/tink-crypto/tink-go/v2/keyset"
	"github.com/tink-crypto/tink-go/v2/mac"

	"github.com/getsphericon/sphericon/internal/secrets"
)

// A well-formed keyset of the wrong primitive (an HMAC key) cannot back an AEAD cipher.
func TestNewCipherRejectsAKeysetThatIsNotAnAEAD(t *testing.T) {
	kh, err := keyset.NewHandle(mac.HMACSHA256Tag256KeyTemplate())
	require.NoError(t, err)
	buf := new(bytes.Buffer)
	require.NoError(t, insecurecleartextkeyset.Write(kh, keyset.NewBinaryWriter(buf)))

	_, err = secrets.NewCipher(base64.StdEncoding.EncodeToString(buf.Bytes()))
	assert.ErrorContains(t, err, "build AEAD")
}

func TestNewCipherTrimsWhitespaceAroundTheKeyset(t *testing.T) {
	ks, err := secrets.GenerateKeysetBase64()
	require.NoError(t, err)
	c, err := secrets.NewCipher("  " + ks + "\n")
	require.NoError(t, err)
	enc, err := c.Encrypt([]byte("x"))
	require.NoError(t, err)
	got, err := c.Decrypt(enc)
	require.NoError(t, err)
	assert.Equal(t, "x", string(got))
}
