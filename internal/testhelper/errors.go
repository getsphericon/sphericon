package testhelper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ErrorContainsOrNone is the assertion of a table row whose expected error is a text: a row with
// none expects no error, any other expects an error that contains it.
func ErrorContainsOrNone(tb testing.TB, err error, want string) {
	tb.Helper()
	if want == "" {
		assert.NoError(tb, err)
		return
	}
	assert.ErrorContains(tb, err, want)
}
