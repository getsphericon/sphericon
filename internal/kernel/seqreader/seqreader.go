// Package seqreader turns a sequence of chunks (iter.Seq2[[]byte, error]), which a domain module
// yields instead of writing (ADR 0111), into the io.ReadCloser a driver streams: an HTTP
// response's body reads the next chunk only when it has sent the last, so a long export never
// sits in memory.
package seqreader

import (
	"io"
	"iter"
)

// New reads the chunks of seq in turn, then io.EOF, or the error seq ends with. Close stops
// seq where it is; a reader never read starts nothing. A reader is read by one goroutine.
func New(seq iter.Seq2[[]byte, error]) io.ReadCloser {
	return &reader{seq: seq}
}

type reader struct {
	seq  iter.Seq2[[]byte, error]
	next func() ([]byte, error, bool)
	stop func()
	buf  []byte
	err  error
}

func (r *reader) Read(p []byte) (int, error) {
	for len(r.buf) == 0 {
		if r.err != nil {
			return 0, r.err
		}
		if r.next == nil {
			r.next, r.stop = iter.Pull2(r.seq)
		}
		chunk, err, ok := r.next()
		switch {
		case !ok:
			r.err = io.EOF
		case err != nil:
			r.err = err
		default:
			r.buf = chunk
		}
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

func (r *reader) Close() error {
	if r.stop != nil {
		r.stop()
	}
	return nil
}
