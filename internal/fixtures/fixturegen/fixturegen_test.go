package fixturegen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures/fixturegen"
)

func writeFixtures(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	return dir
}

func TestGenerateEmitsConstantsForAnnotatedRowsOnly(t *testing.T) {
	dir := writeFixtures(t, map[string]string{
		"workspaces.yml": "- id: 1 # fixture: Acme\n  public_id: acme\n  collect_key: ck_1\n- id: 2\n  public_id: other\n",
		"contacts.yml":   "- id: 7 # fixture: ContactAlice\n  email: alice@example.com\n  created_at: '{{daysAgo 1}}'\n- id: 8\n  email: bob@example.com\n",
	})

	src, err := fixturegen.Generate(dir)
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "Code generated")
	require.Contains(t, out, "AcmeID")
	require.Contains(t, out, "AcmePublicID")
	require.Contains(t, out, `"acme"`)
	require.Contains(t, out, "AcmeCollectKey")
	require.Contains(t, out, "ContactAliceID")
	require.Contains(t, out, "ContactAliceEmail")
	require.NotContains(t, out, "other")
	require.NotContains(t, out, "bob@example.com")
}

func TestGenerateExtractsPlaintextOfCredentialTemplateCalls(t *testing.T) {
	dir := writeFixtures(t, map[string]string{
		"users.yml": "- id: 1 # fixture: Owner\n  email: o@example.com\n  password_hash: '{{argonHash \"owner-pw\"}}'\n",
		"tokens.yml": "- id: 1 # fixture: Tok\n  prefix: abc\n  secret_hash: '{{ bcryptHash \"tok-secret\" }}'\n" +
			"- id: 2 # fixture: Static\n  prefix: def\n  secret_hash: $2a$12$staticstatichash\n",
	})

	src, err := fixturegen.Generate(dir)
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, `OwnerPassword = "owner-pw"`)
	require.Contains(t, out, `TokSecret = "tok-secret"`)
	require.NotContains(t, out, "StaticSecret")
	require.NotContains(t, out, "staticstatichash")
}

func TestGenerateExtractsTheSecondFactorSecretAndRecoveryCode(t *testing.T) {
	dir := writeFixtures(t, map[string]string{
		"users.yml": "- id: 1 # fixture: Sam\n  email: s@example.com\n" +
			"  second_factor_secret_encrypted: '{{encrypt \"JBSWY3DPEHPK3PXP\"}}'\n",
		"recovery_codes.yml": "- id: 1 # fixture: SamRecovery\n  user_id: 1\n  code_hash: '{{recoveryCodeHash \"abcde-fghij\"}}'\n",
		"integrations.yml":   "- id: 1 # fixture: Smtp\n  config_encrypted: '{{encrypt \"{}\"}}'\n",
	})

	src, err := fixturegen.Generate(dir)
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, `SamTotpSecret = "JBSWY3DPEHPK3PXP"`)
	require.Contains(t, out, `SamRecoveryCode = "abcde-fghij"`)
	require.NotContains(t, out, "SmtpTotpSecret", "only the second factor column is lifted")
}

func TestGenerateFailsOnDuplicateName(t *testing.T) {
	dir := writeFixtures(t, map[string]string{
		"workspaces.yml": "- id: 1 # fixture: Dup\n  public_id: a\n",
		"contacts.yml":   "- id: 2 # fixture: Dup\n  email: a@example.com\n",
	})

	_, err := fixturegen.Generate(dir)
	require.ErrorContains(t, err, "duplicate")
}

func TestGenerateFailsOnAnnotationMatchingNoRow(t *testing.T) {
	dir := writeFixtures(t, map[string]string{
		"workspaces.yml": "# fixture: Orphan\n- id: 1\n  public_id: a\n",
	})

	_, err := fixturegen.Generate(dir)
	require.ErrorContains(t, err, "Orphan")
}
