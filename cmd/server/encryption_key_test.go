package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/rekey"
	"github.com/getsphericon/sphericon/internal/secrets"
)

type fakeRekeyOps struct {
	counts []rekey.Count
	err    error
	runs   int
}

func (f *fakeRekeyOps) Reencrypt(context.Context) ([]rekey.Count, error) {
	f.runs++
	return f.counts, f.err
}

func keyDeps(keyset string) deps {
	return deps{env: "test", encryptionKey: func() (string, error) { return keyset, nil }}
}

func mustCipher(t *testing.T, keyset string) *secrets.Cipher {
	t.Helper()
	c, err := secrets.NewCipher(keyset)
	require.NoError(t, err)
	return c
}

func TestEncryptionKeyRotatePrintsOnlyTheNewKeysetOnStdout(t *testing.T) {
	old, err := secrets.GenerateKeysetBase64()
	require.NoError(t, err)
	enc, err := mustCipher(t, old).Encrypt([]byte("x"))
	require.NoError(t, err)
	var out bytes.Buffer

	require.NoError(t, runDeps(keyDeps(old), &out, "encryption-key", "rotate"))

	rotated := mustCipher(t, strings.TrimSpace(out.String()))
	assert.Equal(t, 1, strings.Count(out.String(), "\n"), "the keyset and nothing else")
	got, err := rotated.Decrypt(enc)
	require.NoError(t, err)
	assert.Equal(t, "x", string(got), "the old key stays for decryption")
	current, err := rotated.Current(enc)
	require.NoError(t, err)
	assert.False(t, current, "the new key is the primary")
}

func TestEncryptionKeyRetireKeepsOnlyThePrimary(t *testing.T) {
	old, err := secrets.GenerateKeysetBase64()
	require.NoError(t, err)
	rotated, err := secrets.Rotate(old)
	require.NoError(t, err)
	enc, err := mustCipher(t, old).Encrypt([]byte("x"))
	require.NoError(t, err)
	var out bytes.Buffer

	require.NoError(t, runDeps(keyDeps(rotated), &out, "encryption-key", "retire"))

	_, err = mustCipher(t, strings.TrimSpace(out.String())).Decrypt(enc)
	assert.ErrorIs(t, err, secrets.ErrCiphertext)
}

func TestEncryptionKeyCommandsNeedAKeyset(t *testing.T) {
	for _, sub := range []string{"rotate", "retire"} {
		err := runDeps(keyDeps(""), &bytes.Buffer{}, "encryption-key", sub)
		require.ErrorContains(t, err, "SPHERICON_ENCRYPTION_KEY is not set", sub)
	}
	failing := deps{env: "test", encryptionKey: func() (string, error) { return "", errors.New("boom") }}
	assert.ErrorContains(t, runDeps(failing, &bytes.Buffer{}, "encryption-key", "rotate"), "boom")
}

func TestEncryptionKeyReencryptReportsCountsAndSaysWhenToRunAgain(t *testing.T) {
	ops := &fakeRekeyOps{counts: []rekey.Count{{Column: "integrations.config_encrypted", Rewritten: 3, Current: 1}}}
	var out bytes.Buffer

	require.NoError(t, execute(ops, &out, "encryption-key", "reencrypt"))

	assert.Equal(t, 1, ops.runs)
	assert.Contains(t, out.String(), "integrations.config_encrypted")
	assert.Contains(t, out.String(), "run it once more")
}

func TestEncryptionKeyReencryptAllCurrentPointsToRetire(t *testing.T) {
	ops := &fakeRekeyOps{counts: []rekey.Count{{Column: "webhook_endpoints.secret_encrypted", Current: 2}}}
	var out bytes.Buffer

	require.NoError(t, execute(ops, &out, "encryption-key", "reencrypt"))

	assert.Contains(t, out.String(), "can be retired")
}

func TestEncryptionKeyReencryptFailsAndStillShowsWhatItDid(t *testing.T) {
	ops := &fakeRekeyOps{
		counts: []rekey.Count{{Column: "integrations.config_encrypted", Rewritten: 3}},
		err:    errors.New("webhook_endpoints.secret_encrypted: row 4: invalid or corrupt ciphertext"),
	}
	var out bytes.Buffer

	err := execute(ops, &out, "encryption-key", "reencrypt")

	require.ErrorContains(t, err, "row 4")
	assert.Contains(t, out.String(), "integrations.config_encrypted")
}

func TestEncryptionKeyNeedsAKnownSubcommand(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}, {"rotate", "extra"}} {
		require.Error(t, execute(&fakeRekeyOps{}, &bytes.Buffer{}, append([]string{"encryption-key"}, args...)...), "args %v", args)
	}
}
