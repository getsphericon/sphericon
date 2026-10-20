package fixturegen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures/fixturegen"
)

func TestWriteGeneratesTheCatalogFile(t *testing.T) {
	dir := writeFixtures(t, map[string]string{
		"workspaces.yml": "- id: 1 # fixture: Acme\n  public_id: acme\n",
	})
	out := filepath.Join(t.TempDir(), "catalog_gen.go")

	require.NoError(t, fixturegen.Write(dir, out))

	written, err := os.ReadFile(out)
	require.NoError(t, err)
	want, err := fixturegen.Generate(dir)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(written))
	assert.Contains(t, string(written), "AcmeID = 1")
}

func TestWriteDoesNotTouchTheOutputWhenGenerationFails(t *testing.T) {
	dir := writeFixtures(t, map[string]string{"workspaces.yml": "- id: [unterminated\n"})
	out := filepath.Join(t.TempDir(), "catalog_gen.go")

	require.Error(t, fixturegen.Write(dir, out))
	assert.NoFileExists(t, out)
}

func TestWriteReportsAnUnwritableOutput(t *testing.T) {
	dir := writeFixtures(t, map[string]string{"workspaces.yml": "- id: 1 # fixture: Acme\n"})
	err := fixturegen.Write(dir, filepath.Join(t.TempDir(), "missing-dir", "catalog_gen.go"))
	require.Error(t, err)
}

func TestGenerateFailsOnInvalidYAMLAndNamesTheFile(t *testing.T) {
	dir := writeFixtures(t, map[string]string{"broken.yml": "- id: [unterminated\n"})
	_, err := fixturegen.Generate(dir)
	require.ErrorContains(t, err, "broken.yml")
}

func TestGenerateFailsOnDuplicateNameWithinOneFile(t *testing.T) {
	dir := writeFixtures(t, map[string]string{
		"contacts.yml": "- id: 1 # fixture: Twin\n  email: a@example.com\n- id: 2 # fixture: Twin\n  email: b@example.com\n",
	})
	_, err := fixturegen.Generate(dir)
	require.ErrorContains(t, err, `duplicate fixture name "Twin"`)
}

func TestGenerateFailsOnAnUnreadableFixture(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dir.yml"), 0o700))
	_, err := fixturegen.Generate(dir)
	require.Error(t, err)
}

func TestGenerateFailsOnAMalformedDirectoryPattern(t *testing.T) {
	_, err := fixturegen.Generate("[")
	require.Error(t, err)
}

// Files that are not a list of mappings yield no constants rather than an error.
func TestGenerateIgnoresNonRowContent(t *testing.T) {
	dir := writeFixtures(t, map[string]string{
		"empty.yml":      "",
		"mapping.yml":    "key: value\n",
		"scalars.yml":    "- just a scalar\n- {}\n",
		"workspaces.yml": "- id: 1 # fixture: Acme\n  public_id: acme\n",
	})
	src, err := fixturegen.Generate(dir)
	require.NoError(t, err)
	assert.Contains(t, string(src), "AcmeID")
}

// A credential whose literal cannot be unquoted is skipped, not emitted half-parsed.
func TestGenerateSkipsCredentialWithInvalidEscape(t *testing.T) {
	dir := writeFixtures(t, map[string]string{
		"users.yml": "- id: 1 # fixture: Owner\n  email: o@example.com\n  password_hash: '{{argonHash \"bad\\q\"}}'\n",
	})
	src, err := fixturegen.Generate(dir)
	require.NoError(t, err)
	assert.Contains(t, string(src), "OwnerEmail")
	assert.NotContains(t, string(src), "OwnerPassword")
}
