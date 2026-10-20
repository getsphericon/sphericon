package emailrender_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const htmlWithText = `<table><tbody><tr><td>%s</td></tr></tbody></table>`

// A template author must never reach the server's files: {% include %} and
// {% render %} fail the render, in the subject and in the body alike, instead
// of reading from the working directory.
func TestRenderRejectsFileTags(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("TOP-SECRET"), 0o600))
	t.Chdir(dir)

	for _, tag := range []string{`{% include "secret.txt" %}`, `{% render "secret.txt" %}`} {
		t.Run(tag, func(t *testing.T) {
			_, err := emailrender.RenderEmail(tag, sampleHTML, nil)
			require.ErrorContains(t, err, "render subject")
			require.ErrorContains(t, err, "include and render tags are not supported")

			out, err := emailrender.RenderEmail("Hi", htmlWithBody(tag), nil)
			require.ErrorContains(t, err, "render body")
			require.ErrorContains(t, err, "include and render tags are not supported")
			assert.NotContains(t, out.HTML, "TOP-SECRET")

			require.ErrorContains(t, emailrender.Lint("Hi", htmlWithBody(tag), nil).Error, "include and render tags are not supported")
		})
	}

	require.NoError(t, emailrender.Lint("Hi", htmlWithBody("plain text"), nil).Error)
}

func htmlWithBody(text string) string {
	return fmt.Sprintf(htmlWithText, text)
}
