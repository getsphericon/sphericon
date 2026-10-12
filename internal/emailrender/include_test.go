package emailrender_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A template author must never reach the server's files: {% include %} and
// {% render %} fail the render instead of reading from the working directory.
func TestRenderRejectsFileTags(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("TOP-SECRET"), 0o600))
	t.Chdir(dir)

	for _, tag := range []string{`{% include "secret.txt" %}`, `{% render "secret.txt" %}`} {
		t.Run(tag, func(t *testing.T) {
			out, err := emailrender.Render(tag, nil)
			require.Error(t, err)
			assert.NotContains(t, out, "TOP-SECRET")

			require.Error(t, emailrender.Validate("subject", "<mjml><mj-body><mj-section><mj-column><mj-text>"+tag+"</mj-text></mj-column></mj-section></mj-body></mjml>"))
		})
	}
}
