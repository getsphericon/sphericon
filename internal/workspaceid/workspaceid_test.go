package workspaceid_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/workspaceid"
)

func TestNewIsEightBase62CharactersAndDiffersEachTime(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		id := workspaceid.New()
		assert.Regexp(t, workspaceid.Pattern, id)
		seen[id] = true
	}
	assert.Greater(t, len(seen), 190)
}
