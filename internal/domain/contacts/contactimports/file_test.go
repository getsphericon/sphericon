package contactimports_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contactimport"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
)

// A file saved by a spreadsheet with a byte order mark and semicolons is read as a comma file is,
// its quoted cells kept whole and a row of another cell count counted.
func TestAFileWithABOMAndSemicolonsIsRead(t *testing.T) {
	f, err := contactimports.Inspect([]byte("\xEF\xBB\xBFemail;name\n\"a@example.com\";\"Ann; the first\"\nb@example.com\n"))
	require.NoError(t, err)

	assert.Equal(t, contactimport.DelimiterSemicolon, f.Delimiter)
	assert.Equal(t, []string{"email", "name"}, f.Header)
	assert.Equal(t, 2, f.Rows)
	assert.Equal(t, 1, f.Malformed)
	assert.False(t, strings.HasPrefix(string(f.Body), "\xEF\xBB\xBF"))
}

func TestATabFileIsRead(t *testing.T) {
	f, err := contactimports.Inspect([]byte("email\tname\na@example.com\tAnn\n"))
	require.NoError(t, err)

	assert.Equal(t, contactimport.DelimiterTab, f.Delimiter)
	assert.Equal(t, []string{"email", "name"}, f.Header)
}

func TestAFileIsRefusedWhole(t *testing.T) {
	_, latin1 := contactimports.Inspect([]byte("email\nfran\xe7ois@example.com\n"))
	_, empty := contactimports.Inspect([]byte("\xEF\xBB\xBF"))
	_, wide := contactimports.Inspect([]byte(strings.Repeat("c,", bounds.ContactImportColumnsMaxItems) + "c\n"))
	_, quotes := contactimports.Inspect([]byte("email,name\na@example.com,\"Ann\n"))

	require.ErrorIs(t, latin1, contactimports.ErrNotUTF8)
	require.ErrorIs(t, empty, contactimports.ErrNoHeader)
	require.ErrorIs(t, wide, contactimports.ErrTooManyColumns)
	require.ErrorIs(t, quotes, contactimports.ErrUnparsable)
}

// Whatever bytes are uploaded, the file is read or refused whole, never both, and a file read has a
// header within the bound and no more malformed rows than rows.
func FuzzAFileIsReadOrRefused(f *testing.F) {
	for _, seed := range []string{
		"email,name\na@example.com,Ann\n",
		"\xEF\xBB\xBFemail;name\na@example.com;Ann\n",
		"email\tname\n\"a\tb\"\tc\n",
		"email\nfran\xe7ois@example.com\n",
		"email,name\n\"unterminated\n",
		"",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		file, err := contactimports.Inspect(data)

		assert.True(t, err != nil || len(file.Header) > 0, "a file read has a header")
		assert.True(t, err == nil || file.Header == nil, "a file refused has nothing read")
		assert.LessOrEqual(t, len(file.Header), bounds.ContactImportColumnsMaxItems)
		assert.LessOrEqual(t, file.Malformed, file.Rows)
	})
}
