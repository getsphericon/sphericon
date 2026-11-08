package commandline_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/commandline"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/usecase/secondfactorreset"
)

const resetEmail = "sam@globex.test"

func keysetCipher(t *testing.T, keyset string) *secrets.Cipher {
	t.Helper()
	c, err := secrets.NewCipher(keyset)
	require.NoError(t, err)
	return c
}

func TestReportResetSaysTheFactorIsClearedAndSessionsEnded(t *testing.T) {
	var out bytes.Buffer

	err := commandline.ReportSecondFactorReset(&out, resetEmail, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "the factor and recovery codes are cleared and every session has ended")
}

func TestReportResetOfAUserWithoutASecondFactorSaysNothingChanged(t *testing.T) {
	var out bytes.Buffer

	err := commandline.ReportSecondFactorReset(&out, resetEmail, secondfactorreset.ErrNothingToReset)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "has no second factor; nothing changed")
}

func TestReportResetOfAFailedResetFails(t *testing.T) {
	var out bytes.Buffer
	failed := errors.New(`user "nobody@globex.test": not found`)

	err := commandline.ReportSecondFactorReset(&out, resetEmail, failed)

	require.ErrorIs(t, err, failed)
	assert.Empty(t, out.String())
}
func TestRotateKeepsTheOldKeyForDecryptionAndSaysToReencrypt(t *testing.T) {
	old, err := secrets.GenerateKeysetBase64()
	require.NoError(t, err)
	oldCipher := keysetCipher(t, old)
	sealed, err := oldCipher.Encrypt([]byte("x"))
	require.NoError(t, err)

	change, err := commandline.RotateKeyset(old)

	require.NoError(t, err)
	rotated := keysetCipher(t, change.Keyset)
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
	oldCipher := keysetCipher(t, old)
	sealed, err := oldCipher.Encrypt([]byte("x"))
	require.NoError(t, err)

	change, err := commandline.RetireKeyset(rotated)

	require.NoError(t, err)
	retired := keysetCipher(t, change.Keyset)
	_, err = retired.Decrypt(sealed)
	assert.ErrorIs(t, err, secrets.ErrCiphertext)
}

func TestAKeysetChangeNeedsAKeyset(t *testing.T) {
	_, rotateErr := commandline.RotateKeyset("")
	_, retireErr := commandline.RetireKeyset("")

	require.ErrorIs(t, rotateErr, commandline.ErrNoKeyset)
	require.ErrorIs(t, retireErr, commandline.ErrNoKeyset)
}

func TestReencryptionReportOfARunThatRewroteSaysToRunAgain(t *testing.T) {
	var out bytes.Buffer

	err := commandline.WriteReencryptionReport(&out, []secrets.Count{{Column: "integrations.config_encrypted", Rewritten: 3, Current: 1}}, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "integrations.config_encrypted")
	assert.Contains(t, out.String(), "run it once more")
}

func TestReencryptionReportOfARunThatRacedSaysToRunAgain(t *testing.T) {
	var out bytes.Buffer

	err := commandline.WriteReencryptionReport(&out, []secrets.Count{{Column: "integrations.config_encrypted", Current: 1, Raced: 1}}, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "run it once more")
}

func TestReencryptionReportOfARunThatFoundEverythingCurrentPointsToRetire(t *testing.T) {
	var out bytes.Buffer

	err := commandline.WriteReencryptionReport(&out, []secrets.Count{{Column: "webhook_endpoints.secret_encrypted", Current: 2}}, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "can be retired")
}

func TestReencryptionReportOfARunThatStoppedShowsWhatItDidAndFails(t *testing.T) {
	var out bytes.Buffer
	stopped := errors.New("webhook_endpoints.secret_encrypted: row 4: invalid or corrupt ciphertext")

	err := commandline.WriteReencryptionReport(&out, []secrets.Count{{Column: "integrations.config_encrypted", Rewritten: 3}}, stopped)

	require.ErrorIs(t, err, stopped)
	assert.Contains(t, out.String(), "integrations.config_encrypted")
	assert.NotContains(t, out.String(), "run it once more")
}
