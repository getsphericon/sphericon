package site

import (
	"context"
	"errors"
	"io"
	"iter"
	"log/slog"
	"net/http"

	"github.com/getsphericon/sphericon/internal/kernel/seqreader"
)

type responseKey struct{}

// Responses keeps the response of a /site request within reach of its handler: a download that is
// spent once delivered (delivered) flushes it to learn that the file reached the client.
func Responses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), responseKey{}, http.NewResponseController(w))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// delivered is the body that streams chunks and, once the last was written, flushes the response
// and runs done: a stream cut short, or a flush that fails, leaves done unrun. A response out of
// reach (no Responses before the handler) never runs it.
func delivered(ctx context.Context, chunks iter.Seq2[[]byte, error], done func(context.Context) error) io.ReadCloser {
	return &deliveredReader{ReadCloser: seqreader.New(chunks), ctx: ctx, done: done}
}

type deliveredReader struct {
	io.ReadCloser
	ctx  context.Context
	done func(context.Context) error
	ran  bool
}

// Read reads the next chunk; at the end of the chunks, every one written, it flushes the response
// and runs done once.
func (r *deliveredReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if !errors.Is(err, io.EOF) || r.ran {
		return n, err
	}
	r.ran = true
	rc, ok := r.ctx.Value(responseKey{}).(*http.ResponseController)
	if !ok {
		return n, err
	}
	if flushErr := rc.Flush(); flushErr != nil {
		slog.WarnContext(r.ctx, "a download was not confirmed delivered and is kept", "err", flushErr)
		return n, err
	}
	if doneErr := r.done(r.ctx); doneErr != nil {
		slog.ErrorContext(r.ctx, "a delivered download was not spent", "err", doneErr)
	}
	return n, err
}
