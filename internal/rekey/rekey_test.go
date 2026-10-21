package rekey_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/rekey"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func total(counts []rekey.Count, pick func(rekey.Count) int) int {
	var n int
	for _, c := range counts {
		n += pick(c)
	}
	return n
}

func cipherFor(t *testing.T, keyset string) *secrets.Cipher {
	t.Helper()
	c, err := secrets.NewCipher(keyset)
	require.NoError(t, err)
	return c
}

// The operator's rotation end to end: rotate the keyset, re-encrypt, see a second run do nothing,
// retire the old key, and read every stored secret with what is left.
func TestRotateReencryptAndRetireLeaveEveryStoredSecretReadable(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	cfg, err := config.Load("test")
	require.NoError(t, err)

	// An Enrollment stands on the first version of the Slack Automation (fixture), so editing its
	// steps leaves that version as it was and adds a second: both hold sealed secrets.
	_, err = automations.New(env.Cipher, env.Bus).Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationNotifySlackID, automations.UpdateInput{Steps: &[]automations.Step{
		{Type: automations.StepHTTP, ID: "01J00000000000000000000001", URL: "https://hooks.example.test/v2", Body: `{"v":2}`, Secret: "whsec_second_version"},
	}})
	require.NoError(t, err)
	slackVersions, err := env.DB.AutomationVersion.Query().Where(automationversion.AutomationID(fixtures.AutomationNotifySlackID)).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, slackVersions)

	rotatedKeyset, err := secrets.Rotate(cfg.EncryptionKey)
	require.NoError(t, err)
	rotated := cipherFor(t, rotatedKeyset)

	// The fixtures were sealed under the old key; the rotated keyset still reads them.
	integrations, err := env.DB.Integration.Query().All(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, integrations)
	for _, row := range integrations {
		_, err := rotated.Decrypt(row.ConfigEncrypted)
		require.NoError(t, err, "integration %d under the rotated keyset", row.ID)
	}

	counts, err := rekey.New(env.DB, rotated).Run(ctx)
	require.NoError(t, err)
	assert.Positive(t, total(counts, func(c rekey.Count) int { return c.Rewritten }))
	assert.Zero(t, total(counts, func(c rekey.Count) int { return c.Raced }))
	for _, c := range counts {
		assert.Positive(t, c.Rewritten, "%s holds a fixture that was rewritten", c.Column)
	}

	again, err := rekey.New(env.DB, rotated).Run(ctx)
	require.NoError(t, err)
	assert.Zero(t, total(again, func(c rekey.Count) int { return c.Rewritten }), "a second run is idempotent")
	for i, c := range again {
		assert.GreaterOrEqual(t, c.Current, counts[i].Rewritten, "%s: everything rewritten is current now", c.Column)
	}

	retiredKeyset, err := secrets.Retire(rotatedKeyset)
	require.NoError(t, err)
	retired := cipherFor(t, retiredKeyset)

	integrations, err = env.DB.Integration.Query().All(ctx)
	require.NoError(t, err)
	for _, row := range integrations {
		_, err := retired.Decrypt(row.ConfigEncrypted)
		require.NoError(t, err, "integration %d under the retired keyset", row.ID)
	}
	archivedIntegration := lo.ContainsBy(integrations, func(row *ent.Integration) bool { return row.ID == fixtures.IntegrationAcmeArchivedID })
	assert.True(t, archivedIntegration, "the archived Integration is re-encrypted too")
	withParams := lo.Filter(integrations, func(row *ent.Integration, _ int) bool { return row.ParamsEncrypted != "" })
	for _, row := range withParams {
		_, err := retired.Decrypt(row.ParamsEncrypted)
		require.NoError(t, err, "integration %d params", row.ID)
	}
	webhooks, err := env.DB.WebhookEndpoint.Query().All(ctx)
	require.NoError(t, err)
	for _, row := range webhooks {
		_, err := retired.Decrypt(row.SecretEncrypted)
		require.NoError(t, err, "webhook endpoint %d", row.ID)
	}
	domains, err := env.DB.SendingDomain.Query().All(ctx)
	require.NoError(t, err)
	for _, row := range domains {
		_, err := retired.Decrypt(row.DkimPrivateKeyEncrypted)
		require.NoError(t, err, "sending domain %d", row.ID)
	}
	archivedDomain := lo.ContainsBy(domains, func(row *ent.SendingDomain) bool { return row.ID == fixtures.SendingDomainArchivedID })
	assert.True(t, archivedDomain, "the archived Sending domain is re-encrypted too")
	staff, err := env.DB.StaffMember.Query().All(ctx)
	require.NoError(t, err)
	enrolled := lo.Filter(staff, func(row *ent.StaffMember, _ int) bool { return row.TotpSecretEncrypted != "" })
	for _, row := range enrolled {
		_, err := retired.Decrypt(row.TotpSecretEncrypted)
		require.NoError(t, err, "staff member %d", row.ID)
	}
	users, err := env.DB.User.Query().All(ctx)
	require.NoError(t, err)
	withSecondFactor := lo.Filter(users, func(row *ent.User, _ int) bool { return row.SecondFactorSecretEncrypted != "" })
	for _, row := range withSecondFactor {
		_, err := retired.Decrypt(row.SecondFactorSecretEncrypted)
		require.NoError(t, err, "user %d", row.ID)
	}
	rows, err := env.DB.AutomationVersion.Query().All(ctx)
	require.NoError(t, err)
	var sealedFields int
	for _, row := range rows {
		steps, err := automations.Decode(row.Steps)
		require.NoError(t, err)
		httpSteps := lo.Filter(steps, func(s automations.Step, _ int) bool { return s.Type == automations.StepHTTP })
		for _, s := range httpSteps {
			fields := []string{s.URL, s.Secret}
			for _, h := range s.Headers {
				fields = append(fields, h.Value)
			}
			for _, f := range fields {
				_, err := retired.Decrypt(f)
				require.NoError(t, err, "automation %d HTTP step %s", row.ID, s.ID)
				sealedFields++
			}
		}
	}
	assert.Positive(t, sealedFields, "the fixtures hold HTTP steps")
	slackVersions = lo.CountBy(rows, func(row *ent.AutomationVersion) bool { return row.AutomationID == fixtures.AutomationNotifySlackID })
	assert.Equal(t, 2, slackVersions, "an older version is read along with the latest")

	// The old keyset alone no longer reads the rewritten rows: the rotation took effect.
	_, err = cipherFor(t, cfg.EncryptionKey).Decrypt(integrations[0].ConfigEncrypted)
	assert.ErrorIs(t, err, secrets.ErrCiphertext)
}

// A value the keyset cannot read means a key is missing: the run stops and says where.
func TestReencryptStopsOnAValueNoKeyReads(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	foreign, err := secrets.GenerateKeysetBase64()
	require.NoError(t, err)
	foreignKey := cipherFor(t, foreign)

	counts, err := rekey.New(env.DB, foreignKey).Run(ctx)
	require.ErrorIs(t, err, secrets.ErrCiphertext)
	require.ErrorContains(t, err, "integrations.config_encrypted")
	assert.Empty(t, counts)
}
