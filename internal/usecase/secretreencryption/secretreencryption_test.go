package secretreencryption_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/ruleversion"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func total(counts []secrets.Count, pick func(secrets.Count) int) int {
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
	_, err = automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now).Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationNotifySlackID, automations.UpdateInput{Steps: &[]automationstep.Step{
		{Type: automationstep.HTTP, ID: "01J00000000000000000000001", URL: "https://hooks.example.test/v2", Body: `{"v":2}`, Secret: "whsec_second_version"},
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

	counts, err := reencryption(env, rotated).Run(ctx)
	require.NoError(t, err)
	assert.Positive(t, total(counts, func(c secrets.Count) int { return c.Rewritten }))
	assert.Zero(t, total(counts, func(c secrets.Count) int { return c.Raced }))
	for _, c := range counts {
		assert.Positive(t, c.Rewritten, "%s holds a fixture that was rewritten", c.Column)
	}

	again, err := reencryption(env, rotated).Run(ctx)
	require.NoError(t, err)
	assert.Zero(t, total(again, func(c secrets.Count) int { return c.Rewritten }), "a second run is idempotent")
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
	factors, err := env.DB.SecondFactor.Query().All(ctx)
	require.NoError(t, err)
	withSecondFactor := lo.Filter(factors, func(row *ent.SecondFactor, _ int) bool { return row.SecretEncrypted != "" })
	require.NotEmpty(t, withSecondFactor)
	for _, row := range withSecondFactor {
		_, err := retired.Decrypt(row.SecretEncrypted)
		require.NoError(t, err, "second factor %d", row.ID)
	}
	rows, err := env.DB.AutomationVersion.Query().All(ctx)
	require.NoError(t, err)
	var sealedFields int
	for _, row := range rows {
		steps, err := automationstep.Decode(row.Steps)
		require.NoError(t, err)
		httpSteps := lo.Filter(steps, func(s automationstep.Step, _ int) bool { return s.Type == automationstep.HTTP })
		for _, s := range httpSteps {
			fields := make([]string, 0, 2+len(s.Headers))
			fields = append(fields, s.URL, s.Secret)
			for _, h := range s.Headers {
				fields = append(fields, h.Value)
			}
			present := lo.Filter(fields, func(value string, _ int) bool { return value != "" })
			for _, f := range present {
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

// A Rule version's http Actions are sealed per field too, and a Firing reads its version after the
// Rule is deleted: every version of a deleted Rule, its draft included, reads under the retired key.
func TestReencryptCoversEveryVersionOfADeletedRule(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	cfg, err := config.Load("test")
	require.NoError(t, err)

	// The HTTP Rule (fixture) has a live version with one http Action and an older one with two;
	// a draft with a header and a signing secret makes a third version. The Rule has Firings, so
	// deleting it discards it and keeps every version for them.
	acme := env.DB.Scoped(fixtures.AcmeID)
	module := rules.New(env.Cipher, env.Bus, nil, time.Now)
	_, err = module.Update(ctx, acme, fixtures.RuleHTTPExecutionID, rules.UpdateInput{Actions: &[]rules.Action{{
		Type: rules.ActionHTTP, URL: "https://hooks.example.test/rule", Body: "{}", Secret: "whsec_rule_draft",
		Headers: []httpcall.Header{{Name: "Authorization", Value: "Bearer rule-draft"}},
	}}})
	require.NoError(t, err)
	require.NoError(t, module.Delete(ctx, acme, fixtures.RuleHTTPExecutionID))

	rotatedKeyset, err := secrets.Rotate(cfg.EncryptionKey)
	require.NoError(t, err)
	_, err = reencryption(env, cipherFor(t, rotatedKeyset)).Run(ctx)
	require.NoError(t, err)
	retiredKeyset, err := secrets.Retire(rotatedKeyset)
	require.NoError(t, err)
	opener := rules.New(cipherFor(t, retiredKeyset), env.Bus, nil, time.Now)

	versions, err := env.DB.RuleVersion.Query().Where(ruleversion.RuleID(fixtures.RuleHTTPExecutionID)).All(ctx)
	require.NoError(t, err)
	require.Len(t, versions, 3)
	var opened []httpcall.Opened
	for _, v := range versions {
		actions, err := rules.DecodeActions(v.Actions)
		require.NoError(t, err)
		for _, a := range actions {
			o, err := opener.OpenHTTP(a)
			require.NoError(t, err, "rule version %d http Action %s", v.ID, a.ID)
			opened = append(opened, o)
		}
	}
	assert.Len(t, opened, 4, "the draft's Action, the live one and the older version's two")
	assert.Contains(t, opened, httpcall.Opened{
		URL: "https://hooks.example.test/rule", Secret: "whsec_rule_draft",
		Headers: map[string]string{"Authorization": "Bearer rule-draft"},
	})
}

// A value the keyset cannot read means a key is missing: the run stops and says where.
func TestReencryptStopsOnAValueNoKeyReads(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	foreign, err := secrets.GenerateKeysetBase64()
	require.NoError(t, err)
	foreignKey := cipherFor(t, foreign)

	counts, err := reencryption(env, foreignKey).Run(ctx)
	require.ErrorIs(t, err, secrets.ErrCiphertext)
	require.ErrorContains(t, err, "integrations.config_encrypted")
	assert.Empty(t, counts)
}
