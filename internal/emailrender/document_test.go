package emailrender_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/emaildoc"
	"github.com/getsphericon/sphericon/internal/emailrender"
)

// One runtime serves the whole test process: starting one takes seconds.
var renderer = emaildoc.New()

func TestFromDocumentBuildsAStorableBody(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "emaildoc", "testdata", "docs", "newsletter.json"))
	require.NoError(t, err)

	got, err := emailrender.FromDocument(t.Context(), renderer, nil, doc)
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
			_, err := emailrender.FromDocument(t.Context(), renderer, nil, []byte(doc))
			require.ErrorIs(t, err, emailrender.ErrInvalidContent)
		})
	}
}

func TestFromDocumentChecksTheSubjectToo(t *testing.T) {
	subject := "{% if %}"
	_, err := emailrender.FromDocument(t.Context(), renderer, &subject,
		[]byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}`))
	require.ErrorIs(t, err, emailrender.ErrInvalidContent)
}

// A render that could not run (the request ended, a limit of ours stopped it) says nothing about
// the document: it is not the author's error, so it is never ErrInvalidContent.
func TestFromDocumentSaysARenderThatCouldNotRunIsNotTheDocumentsFault(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := emailrender.FromDocument(ctx, renderer, nil,
		[]byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}`))

	require.ErrorIs(t, err, emailrender.ErrRenderUnavailable)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, emailrender.ErrInvalidContent)
}

var errBoth = errors.New("a body and a doc")

func TestResolveContent(t *testing.T) {
	doc := []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello"}]}]}`)
	body := "<p>plain</p>"

	t.Run("a document is rendered to the body", func(t *testing.T) {
		got, err := emailrender.ResolveContent(t.Context(), renderer, nil, nil, doc, errBoth)
		require.NoError(t, err)
		require.NotNil(t, got.Authored)
		require.NotNil(t, got.Body)
		assert.Contains(t, *got.Body, "Hello")
		assert.Equal(t, got.Authored.Body, *got.Body)
	})
	t.Run("a body is stored on its own", func(t *testing.T) {
		got, err := emailrender.ResolveContent(t.Context(), renderer, nil, &body, nil, errBoth)
		require.NoError(t, err)
		assert.Nil(t, got.Authored)
		require.NotNil(t, got.Body)
		assert.Contains(t, *got.Body, "plain")
	})
	t.Run("neither leaves the content alone", func(t *testing.T) {
		got, err := emailrender.ResolveContent(t.Context(), renderer, nil, nil, nil, errBoth)
		require.NoError(t, err)
		assert.Nil(t, got.Authored)
		assert.Nil(t, got.Body)
	})
	t.Run("both is the caller's refusal", func(t *testing.T) {
		_, err := emailrender.ResolveContent(t.Context(), renderer, nil, &body, doc, errBoth)
		require.ErrorIs(t, err, errBoth)
	})
}
