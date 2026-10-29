package rekey_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/rekey"
	"github.com/getsphericon/sphericon/internal/secrets"
)

func TestRotateKeepsTheOldKeyForDecryptionAndSaysToReencrypt(t *testing.T) {
	old, err := secrets.GenerateKeysetBase64()
	require.NoError(t, err)
	oldCipher := cipherFor(t, old)
	sealed, err := oldCipher.Encrypt([]byte("x"))
	require.NoError(t, err)

	change, err := rekey.Rotate(old)

	require.NoError(t, err)
	rotated := cipherFor(t, change.Keyset)
	got, err := rotated.Decrypt(sealed)
	require.NoError(t, err)
	assert.Equal(t, "x", string(got), "the old key stays for decryption")
	current, err := rotated.Current(sealed)
	require.NoError(t, err)
	assert.False(t, current, "the new key is the primary")
	assert.Contains(t, change.Next, "sphericon encryption-key reencrypt")
}

func TestRetireKeepsOnlyThePrimary(t *testing.T) {
	old, err := secrets.GenerateKeysetBase64()
	require.NoError(t, err)
	rotated, err := secrets.Rotate(old)
	require.NoError(t, err)
	oldCipher := cipherFor(t, old)
	sealed, err := oldCipher.Encrypt([]byte("x"))
	require.NoError(t, err)

	change, err := rekey.Retire(rotated)

	require.NoError(t, err)
	retired := cipherFor(t, change.Keyset)
	_, err = retired.Decrypt(sealed)
	assert.ErrorIs(t, err, secrets.ErrCiphertext)
}

func TestAKeysetChangeNeedsAKeyset(t *testing.T) {
	_, rotateErr := rekey.Rotate("")
	_, retireErr := rekey.Retire("")

	require.ErrorIs(t, rotateErr, rekey.ErrNoKeyset)
	require.ErrorIs(t, retireErr, rekey.ErrNoKeyset)
}

func TestReportOfARunThatRewroteSaysToRunAgain(t *testing.T) {
	var out bytes.Buffer

	err := rekey.WriteReport(&out, []rekey.Count{{Column: "integrations.config_encrypted", Rewritten: 3, Current: 1}}, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "integrations.config_encrypted")
	assert.Contains(t, out.String(), "run it once more")
}

func TestReportOfARunThatRacedSaysToRunAgain(t *testing.T) {
	var out bytes.Buffer

	err := rekey.WriteReport(&out, []rekey.Count{{Column: "integrations.config_encrypted", Current: 1, Raced: 1}}, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "run it once more")
}

func TestReportOfARunThatFoundEverythingCurrentPointsToRetire(t *testing.T) {
	var out bytes.Buffer

	err := rekey.WriteReport(&out, []rekey.Count{{Column: "webhook_endpoints.secret_encrypted", Current: 2}}, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "can be retired")
}

func TestReportOfARunThatStoppedShowsWhatItDidAndFails(t *testing.T) {
	var out bytes.Buffer
	stopped := errors.New("webhook_endpoints.secret_encrypted: row 4: invalid or corrupt ciphertext")

	err := rekey.WriteReport(&out, []rekey.Count{{Column: "integrations.config_encrypted", Rewritten: 3}}, stopped)

	require.ErrorIs(t, err, stopped)
	assert.Contains(t, out.String(), "integrations.config_encrypted")
	assert.NotContains(t, out.String(), "run it once more")
}
