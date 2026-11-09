package seqreader_test

import (
	"errors"
	"io"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/seqreader"
)

// chunks yields each chunk, then ends with end; produced counts the chunks it handed out and
// stopped says whether its reader stopped it before the end.
type chunks struct {
	items    []string
	end      error
	produced int
	stopped  bool
}

func (c *chunks) seq() iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		for _, item := range c.items {
			c.produced++
			if !yield([]byte(item), nil) {
				c.stopped = true
				return
			}
		}
		if c.end != nil {
			yield(nil, c.end)
		}
	}
}

func TestTheReaderReadsTheChunksInTurn(t *testing.T) {
	c := &chunks{items: []string{`{"a":`, "", `[1,2]`, `}`}}
	r := seqreader.New(c.seq())

	got, err := io.ReadAll(r)

	require.NoError(t, err)
	assert.JSONEq(t, `{"a":[1,2]}`, string(got))
	require.NoError(t, r.Close())
}

func TestTheReaderFailsWithTheErrorTheSequenceEndsWith(t *testing.T) {
	failed := errors.New("page 3: connection reset")
	r := seqreader.New((&chunks{items: []string{"row 1\n"}, end: failed}).seq())

	got, err := io.ReadAll(r)

	require.ErrorIs(t, err, failed)
	assert.Equal(t, "row 1\n", string(got), "what came before the error is read")
}

func TestClosingTheReaderStopsTheSequence(t *testing.T) {
	c := &chunks{items: []string{"a", "b", "c"}}
	r := seqreader.New(c.seq())
	_, err := r.Read(make([]byte, 1))
	require.NoError(t, err)

	require.NoError(t, r.Close())

	assert.True(t, c.stopped)
	assert.Equal(t, 1, c.produced, "nothing after the chunk it was stopped at")
}

func TestAReaderNeverReadStartsNothing(t *testing.T) {
	c := &chunks{items: []string{"a"}}
	r := seqreader.New(c.seq())

	require.NoError(t, r.Close())

	assert.Zero(t, c.produced)
}
