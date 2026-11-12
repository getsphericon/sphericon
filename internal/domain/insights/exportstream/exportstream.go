// Package exportstream is what every Export stream shares (ADR 0045, GLOSSARY: Export
// stream): the opaque (updated_at, id) cursor, the keyset query predicates, the page
// size, the NDJSON page encoder and the check of an encoded row against the TypeSpec
// row schema. A stream adds only its row type (with Position), its fetch query and its
// operation; see events.go for the smallest example.
//
// Core code: Streams asks the Edition whether the `export` license is on (and the Audit log
// whether the `audit` one is) before it serves a stream; without it a stream is ErrNotOffered,
// which every surface maps once (review rule 15).
package exportstream

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/internal/domain/insights/auditlog"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ErrBadCursor means a cursor token is not one this package issued: a malformed or foreign
// value is refused (an unknown reference) instead of silently restarting a consumer from zero.
var ErrBadCursor = validation.UnknownReference("exportstream: invalid cursor", i18n.ErrorsExportCursorInvalid)

// ErrNotOffered means the Instance does not offer the Export streams: without the `export`
// license the feature is absent, like /staff, and a stream is not found (ADR 0045).
var ErrNotOffered = validation.NotFound("exportstream: the export streams are not offered", i18n.ErrorsNotFound)

// License is the Edition's answer whether the `export` feature is licensed (ADR 0045).
type License interface {
	Licensed() bool
}

// Streams serves the Export streams of a Workspace behind their licenses.
type Streams struct {
	export License
	audit  *auditlog.Log
}

// New builds the streams over the `export` license and the Audit log, whose own license gates
// the Audit stream.
func New(export License, audit *auditlog.Log) *Streams {
	return &Streams{export: export, audit: audit}
}

// offered reports whether the Instance offers the Export streams.
func (st *Streams) offered() bool { return st != nil && st.export != nil && st.export.Licensed() }

// auditOffered reports whether it offers the Audit stream: the `audit` license besides `export`.
func (st *Streams) auditOffered() bool { return st.offered() && st.audit.Licensed() }

// Cursor is a position in a stream: every row at or after (At, ID) in (updated_at, id)
// order. The zero Cursor is the beginning of the stream. It is inclusive, and the cursor a
// page hands back is the position right after its last row (Following), so a row that
// shares its updated_at with the last row of a page is never skipped and never repeated.
type Cursor struct {
	At time.Time
	ID int64
}

// IsZero reports whether c is the beginning of the stream.
func (c Cursor) IsZero() bool { return c.At.IsZero() && c.ID == 0 }

// Following is the position right after c: ids are integers, so the successor of
// (updated_at, id) among the rows of one instant is (updated_at, id+1). The instant is cut
// to microseconds, the precision Postgres stores.
func (c Cursor) Following() Cursor {
	return Cursor{At: c.At.Truncate(time.Microsecond), ID: c.ID + 1}
}

// String is the opaque token of c, empty for the beginning of the stream.
func (c Cursor) String() string {
	if c.IsZero() {
		return ""
	}
	raw := strconv.FormatInt(c.At.UnixMicro(), 10) + ":" + strconv.FormatInt(c.ID, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// ParseCursor reads a token String issued. An empty token is the beginning of the stream;
// anything else that String could not have produced is ErrBadCursor.
func ParseCursor(token string) (Cursor, error) {
	if token == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Cursor{}, ErrBadCursor
	}
	micros, id, ok := strings.Cut(string(raw), ":")
	if !ok {
		return Cursor{}, ErrBadCursor
	}
	at, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return Cursor{}, ErrBadCursor
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n < 1 {
		return Cursor{}, ErrBadCursor
	}
	return Cursor{At: time.UnixMicro(at), ID: n}, nil
}

// Seek is the keyset predicate of a stream's query: rows at or after the cursor. Pass it
// to an ent query's Where as a predicate of that entity (`predicate.Event(exportstream.Seek(c))`);
// it adds nothing for the beginning of the stream. It reads the entity's `updated_at`
// and `id` columns, which the index on (workspace_id, updated_at, id) covers.
func Seek(after Cursor) func(*sql.Selector) {
	return func(s *sql.Selector) {
		if after.IsZero() {
			return
		}
		// raw sql: a keyset row-value comparison (updated_at, id) >= (...); ent has no tuple
		// comparison.
		s.Where(sql.P(func(b *sql.Builder) {
			b.WriteString("(" + s.C(columnUpdatedAt) + ", " + s.C(columnID) + ") >= (")
			b.Arg(after.At)
			b.Comma()
			b.Arg(after.ID)
			b.WriteByte(')')
		}))
	}
}

// columnUpdatedAt and columnID are the columns every streamed table keys its position by: the
// change time and the id ent gives every entity.
const (
	columnUpdatedAt = "updated_at"
	columnID        = "id"
)

// Order is the order of a stream's query: oldest change first, ties by id.
func Order(s *sql.Selector) {
	s.OrderBy(sql.Asc(s.C(columnUpdatedAt)), sql.Asc(s.C(columnID)))
}

// Row is one line of a stream: a flat struct whose JSON is the TypeSpec row schema, and
// which knows its own position.
type Row interface {
	// Position is the row's own (updated_at, id).
	Position() Cursor
}

// Page is one answer of a stream: the body (NDJSON, one row per line, no envelope) and the
// token of the cursor for the next request.
type Page struct {
	Cursor string
	Body   io.Reader
}

// Read serves one page of a stream. token is the request's cursor; fetch returns the rows
// at or after a cursor in (updated_at, id) order, at most limit of them. The page's cursor
// is the position after the last row, or token itself when the page is empty, so a poller
// can loop on it without special cases. A malformed token is ErrBadCursor.
func Read[R Row](token string, limit int, fetch func(after Cursor, limit int) ([]R, error)) (Page, error) {
	after, err := ParseCursor(token)
	if err != nil {
		return Page{}, err
	}
	rows, err := fetch(after, limit)
	if err != nil {
		return Page{}, err
	}
	var body bytes.Buffer
	enc := json.NewEncoder(&body)
	enc.SetEscapeHTML(false)
	next := token
	for _, row := range rows {
		if err := enc.Encode(row); err != nil {
			return Page{}, fmt.Errorf("exportstream: encode row: %w", err)
		}
		next = row.Position().Following().String()
	}
	return Page{Cursor: next, Body: &body}, nil
}
