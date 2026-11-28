package attachment_test

import (
	"mime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/attachment"
)

// A download's Content-Disposition names its file as a browser saves it, a name with a quote or a
// character outside ASCII included.
func TestAnAttachmentNamesItsFile(t *testing.T) {
	for _, name := range []string{"audit-log-Ab12.csv", `say "hi".csv`, "Kontakte-Müller.csv"} {
		disposition, params, err := mime.ParseMediaType(attachment.ContentDisposition(name))

		require.NoError(t, err)
		assert.Equal(t, "attachment", disposition)
		assert.Equal(t, name, params["filename"])
	}
}
