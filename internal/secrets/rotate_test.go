package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cipherOf(t *testing.T, ks string) *Cipher {
	t.Helper()
	c, err := NewCipher(ks)
	require.NoError(t, err)
	return c
}

func TestRotateKeepsOldCiphertextReadableAndSealsUnderTheNewKey(t *testing.T) {
	oldKS, err := GenerateKeysetBase64()
	require.NoError(t, err)
	oldCipher := cipherOf(t, oldKS)
	oldSealed, err := oldCipher.Encrypt([]byte("old"))
	require.NoError(t, err)

	newKS, err := Rotate(oldKS)
	require.NoError(t, err)
	assert.NotEqual(t, oldKS, newKS)
	rotated := cipherOf(t, newKS)

	got, err := rotated.Decrypt(oldSealed)
	require.NoError(t, err)
	assert.Equal(t, "old", string(got))

	newSealed, err := rotated.Encrypt([]byte("new"))
	require.NoError(t, err)
	_, err = oldCipher.Decrypt(newSealed)
	assert.ErrorIs(t, err, ErrCiphertext, "the new primary key is not in the old keyset")
}

func TestRewrapMovesOldCiphertextToThePrimaryKeyOnce(t *testing.T) {
	oldKS, err := GenerateKeysetBase64()
	require.NoError(t, err)
	oldSealed, err := cipherOf(t, oldKS).Encrypt([]byte("payload"))
	require.NoError(t, err)
	newKS, err := Rotate(oldKS)
	require.NoError(t, err)
	c := cipherOf(t, newKS)

	current, err := c.Current(oldSealed)
	require.NoError(t, err)
	assert.False(t, current)

	sealed, changed, err := c.Rewrap(oldSealed)
	require.NoError(t, err)
	assert.True(t, changed)
	current, err = c.Current(sealed)
	require.NoError(t, err)
	assert.True(t, current)

	again, changed, err := c.Rewrap(sealed)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, sealed, again)
}

func TestRetiredKeysetReadsRewrappedValuesAndRefusesTheRest(t *testing.T) {
	oldKS, err := GenerateKeysetBase64()
	require.NoError(t, err)
	oldSealed, err := cipherOf(t, oldKS).Encrypt([]byte("payload"))
	require.NoError(t, err)
	newKS, err := Rotate(oldKS)
	require.NoError(t, err)
	rewrapped, _, err := cipherOf(t, newKS).Rewrap(oldSealed)
	require.NoError(t, err)

	retiredKS, err := Retire(newKS)
	require.NoError(t, err)
	retired := cipherOf(t, retiredKS)

	got, err := retired.Decrypt(rewrapped)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(got))
	_, err = retired.Decrypt(oldSealed)
	assert.ErrorIs(t, err, ErrCiphertext)
}

func TestCurrentRejectsAValueWithNoKeyId(t *testing.T) {
	_, err := testCipher(t).Current("AAAA")
	assert.ErrorIs(t, err, ErrCiphertext)
}
