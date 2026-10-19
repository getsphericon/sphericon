package emailrender_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromDocumentBuildsAStorableBody(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "emaildoc", "testdata", "docs", "newsletter.json"))
	require.NoError(t, err)

	got, err := emailrender.FromDocument(t.Context(), nil, doc)
	require.NoError(t, err)

	assert.Equal(t, emailrender.SanitizeHTML(got.Body), got.Body, "the stored body survives the sanitizer")
	assert.NotEmpty(t, got.EditorVersion)
	assert.NotContains(t, got.Body, "<!--$-->")
	for _, tag := range []string{"{{ first_name }}", "{{ contact.id }}", "{{ token | url_encode }}", "{{ cta_url }}"} {
		assert.Contains(t, got.Body, tag)
	}
	require.NoError(t, emailrender.Lint("", got.Body, nil).Error)
}

func TestFromDocumentRefusesWhatCannotBeRendered(t *testing.T) {
	for name, doc := range map[string]string{
		"not a document":     `{"type":"nope"}`,
		"heading of level 9": `{"type":"doc","content":[{"type":"heading","attrs":{"level":9},"content":[{"type":"text","text":"x"}]}]}`,
		"not JSON":           `[`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := emailrender.FromDocument(t.Context(), nil, []byte(doc))
			require.ErrorIs(t, err, emailrender.ErrInvalidContent)
		})
	}
}

func TestFromDocumentChecksTheSubjectToo(t *testing.T) {
	subject := "{% if %}"
	_, err := emailrender.FromDocument(t.Context(), &subject,
		[]byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}`))
	require.ErrorIs(t, err, emailrender.ErrInvalidContent)
}
