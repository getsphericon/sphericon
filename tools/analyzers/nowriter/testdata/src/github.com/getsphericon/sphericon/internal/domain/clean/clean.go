package clean

import (
	"bytes"
	"context"
	"io"
	"iter"
)

// Rows yields what a driver writes.
func Rows(ctx context.Context) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		var buf bytes.Buffer
		buf.WriteString("row")
		yield(buf.Bytes(), nil)
	}
}

// Read takes a reader, which is no writer.
func Read(r io.Reader) ([]byte, error) { return io.ReadAll(r) }
