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
	finish := func() {
		rc, ok := ctx.Value(responseKey{}).(*http.ResponseController)
		if !ok {
			return
		}
		if err := rc.Flush(); err != nil {
			slog.WarnContext(ctx, "a download was not confirmed delivered and is kept", "err", err)
			return
		}
		if err := done(ctx); err != nil {
			slog.ErrorContext(ctx, "a delivered download was not spent", "err", err)
		}
	}
	return &deliveredReader{ReadCloser: seqreader.New(chunks), finish: finish}
}

type deliveredReader struct {
	io.ReadCloser

	// finish confirms the delivery once every chunk was written.
	finish func()
	ran    bool
}

// Read reads the next chunk; at the end of the chunks, every one written, it finishes once.
func (r *deliveredReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if errors.Is(err, io.EOF) && !r.ran {
		r.ran = true
		r.finish()
	}
	return n, err
}
