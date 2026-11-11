package randtext_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/kernel/randtext"
)

func TestFromAlphabetDrawsTheAskedLengthFromTheWholeAlphabetOnly(t *testing.T) {
	seen := map[rune]bool{}
	for range 200 {
		text := randtext.FromAlphabet("xyz", 5)
		assert.Regexp(t, `^[xyz]{5}$`, text)
		for _, c := range text {
			seen[c] = true
		}
	}
	assert.Equal(t, map[rune]bool{'x': true, 'y': true, 'z': true}, seen, "every character turns up")
}

func TestFromAlphabetOfNoCharactersIsEmpty(t *testing.T) {
	assert.Empty(t, randtext.FromAlphabet("xyz", 0))
}
