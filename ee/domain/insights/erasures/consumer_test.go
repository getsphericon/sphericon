package erasures_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee/domain/insights/erasures"
	eejobs "github.com/getsphericon/sphericon/ee/jobs"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/erasure"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/secretreencryption"
)

// publishErased publishes the contact.erased of an Erasure in Initech.
func publishErased(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	require.NoError(t, env.Bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(t.Context(), &events.ContactErased{
			WorkspaceID: fixtures.InitechID, SubjectID: "user-erased", IdentifierKind: "contact_id", RequesterKind: "user", RequesterID: 4,
		})
	}))
}

// countErasures counts Initech's Erasures of the subject publishErased erases.
func countErasures(t *testing.T, env *testhelper.TestEnv) int {
	t.Helper()
	n, err := env.DB.Erasure.Query().Where(erasure.WorkspaceID(fixtures.InitechID), erasure.SubjectID("user-erased")).Count(t.Context())
	require.NoError(t, err)
	return n
}

// An Erasure's contact.erased inserts one export job; working it adds the Erasure to the stream
// once, also when river retries the job.
func TestAnErasureReachesTheStreamOnceThroughItsJob(t *testing.T) {
	env := testhelper.Setup(t)
	publishErased(t, env)
	queued := env.EventJobs(t, eejobs.ErasureEventArgs{}.Kind())
	require.Len(t, queued, 1)

	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
	env.RetryJobs(t, testhelper.EditionJobs...)
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	stored := countErasures(t, env)
	assert.Equal(t, 1, stored)
}

// Without the export license the job is worked and stores nothing (ADR 0045).
func TestAnUnlicensedInstanceStreamsNoErasure(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	publishErased(t, env)
	queued := env.EventJobs(t, eejobs.ErasureEventArgs{}.Kind())
	require.Len(t, queued, 1, "the job is queued whatever the license")

	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	stored := countErasures(t, env)
	assert.Zero(t, stored)
}

// An Erasure's hashes are stored sealed (ADR 0045): the row holds no hash in the clear, which a
// phone's would give away to anyone who tries every number, and opens to it again after the keyset
// is rotated, the rows re-encrypted and the old key retired.
func TestAnErasuresHashesAreStoredSealedAndSurviveARotation(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	sum := sha256.Sum256([]byte("+14155550100"))
	phoneHash := hex.EncodeToString(sum[:])
	require.NoError(t, env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, &events.ContactErased{
			WorkspaceID: fixtures.InitechID, SubjectID: "user-sealed", IdentifierKind: "phone", RequesterKind: "user", RequesterID: 4,
			PhoneHash: phoneHash,
		})
	}))
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
	stored := func() string {
		row, err := env.DB.Erasure.Query().Where(erasure.SubjectID("user-sealed")).Only(ctx)
		require.NoError(t, err)
		require.NotNil(t, row.PhoneHashEncrypted)
		return *row.PhoneHashEncrypted
	}
	assert.NotContains(t, stored(), phoneHash, "the database holds ciphertext only")
	opened, err := env.Cipher.Decrypt(stored())
	require.NoError(t, err)
	assert.Equal(t, phoneHash, string(opened))

	cfg, err := config.Load(config.EnvTest)
	require.NoError(t, err)
	rotatedKeyset, err := secrets.Rotate(cfg.EncryptionKey)
	require.NoError(t, err)
	rotated, err := secrets.NewCipher(rotatedKeyset)
	require.NoError(t, err)
	_, err = secretreencryption.New(erasures.SecretSweeps(env.DB), rotated).Run(ctx)
	require.NoError(t, err)
	again, err := secretreencryption.New(erasures.SecretSweeps(env.DB), rotated).Run(ctx)
	require.NoError(t, err)
	for _, c := range again {
		assert.Zero(t, c.Rewritten, "%s: a second run rewrites nothing", c.Column)
	}
	retiredKeyset, err := secrets.Retire(rotatedKeyset)
	require.NoError(t, err)
	retired, err := secrets.NewCipher(retiredKeyset)
	require.NoError(t, err)
	opened, err = retired.Decrypt(stored())
	require.NoError(t, err)
	assert.Equal(t, phoneHash, string(opened))
}
