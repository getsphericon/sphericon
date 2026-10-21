package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/ThreeDotsLabs/watermill"
	watermillsql "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/trackinghostlog"
	"github.com/getsphericon/sphericon/internal/workspacefeed"
	"github.com/oklog/ulid/v2"
)

// outboxSchema / outboxOffsets are the single source of truth for the
// domain_events topic layout. The producer (WithinTx publisher) and the subscribers
// (NewSubscriber) reference these, so the table the producer writes and the table the
// consumers read cannot drift apart. The tables themselves come from the migrations
// (migrations/*_domain_events_outbox.sql), which mirror this layout.
var (
	outboxSchema  = watermillsql.DefaultPostgreSQLSchema{}
	outboxOffsets = watermillsql.DefaultPostgreSQLOffsetsAdapter{}
)

// Publisher publishes a typed domain event onto the bus. The implementation
// handed to Bus.WithinTx writes into the same transaction as the state change,
// so the event is published iff the state commits (transactional outbox).
type Publisher interface {
	Publish(ctx context.Context, ev DomainEvent) error
}

// Bus is the producer-side entry point. It owns the database pool and opens a
// transaction per WithinTx call that carries both the ent writes and the outbox
// insert.
type Bus struct {
	db     *sql.DB
	logger watermill.LoggerAdapter
	hooks  []ent.Hook
	jobs   JobInserter
}

// New builds a Bus over the application's database pool.
func New(db *sql.DB) *Bus {
	return &Bus{db: db, logger: watermill.NewSlogLogger(slog.Default())}
}

// Use registers ent hooks on the transaction client of every later WithinTx, the way
// Client.Use does for a long-lived client. Register at composition time, before the
// bus is shared with concurrent callers; tests use it to inject faults into a
// transaction the bus opens itself.
func (b *Bus) Use(hooks ...ent.Hook) {
	b.hooks = append(b.hooks, hooks...)
}

// WithinTx runs fn inside a single SQL transaction. The *ent.Client handed to fn
// is bound to that transaction, and so is the Publisher: any event published via
// pub is inserted into the outbox table in the same tx. The tx commits iff fn
// returns nil — guaranteeing state and event commit (or roll back) atomically.
//
// In tests this rides go-txdb's savepoint nesting, so the commit here is a
// RELEASE SAVEPOINT contained by the per-test rollback.
func (b *Bus) WithinTx(ctx context.Context, fn func(tx *ent.Client, pub Publisher) error) error {
	return b.within(ctx, func(tx *ent.Client, pub *txPublisher) error { return fn(tx, pub) })
}

// within is WithinTx over the concrete transaction publisher, which also implements
// QueuePurger (see WithinScopedPurgeTx).
func (b *Bus) within(ctx context.Context, fn func(tx *ent.Client, pub *txPublisher) error) (err error) {
	sqlTx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = sqlTx.Rollback()
			panic(p)
		}
	}()

	// ent client bound to the same *sql.Tx the outbox publisher uses.
	txClient := ent.NewClient(ent.Driver(newTxDriver(sqlTx)))
	txClient.Use(workspacefeed.Hook, trackinghostlog.Hook)
	txClient.Use(b.hooks...)

	// watermill-sql forbids AutoInitializeSchema on a tx handle (a CREATE TABLE
	// would implicitly commit). The outbox tables are created by the migrations.
	wm, err := watermillsql.NewPublisher(watermillsql.TxFromStdSQL(sqlTx), watermillsql.PublisherConfig{
		SchemaAdapter:        outboxSchema,
		AutoInitializeSchema: false,
	}, b.logger)
	if err != nil {
		_ = sqlTx.Rollback()
		return fmt.Errorf("outbox publisher: %w", err)
	}

	if err := fn(txClient, &txPublisher{wm: wm, tx: sqlTx, client: txClient}); err != nil {
		_ = sqlTx.Rollback()
		return err
	}
	if err := sqlTx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// newTxDriver builds an ent driver that runs on an already-open *sql.Tx. ent's
// default driver opens a real nested transaction for multi-statement mutations
// (UpdateNode etc.) by calling BeginTx on the underlying *sql.DB — which panics
// when the connection is a *sql.Tx. This driver instead returns a no-op nested
// transaction that reuses the same tx; the real commit/rollback is owned by
// Bus.WithinTx, which rolls the whole tx back on any error. (Canonical ent
// "bring-your-own-transaction" pattern.)
func newTxDriver(tx *sql.Tx) dialect.Driver {
	return &txDriver{conn: entsql.Conn{ExecQuerier: tx}}
}

type txDriver struct {
	conn entsql.Conn
}

func (d *txDriver) Exec(ctx context.Context, query string, args, v any) error {
	return d.conn.Exec(ctx, query, args, v)
}
func (d *txDriver) Query(ctx context.Context, query string, args, v any) error {
	return d.conn.Query(ctx, query, args, v)
}

// ExecContext lets the client's raw ExecContext (ent's sql/execquery) run on the
// transaction, as internal/workspacefeed needs.
func (d *txDriver) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	// raw sql: forwards the caller's statement to the transaction unchanged; it adds none of its own.
	return d.conn.ExecContext(ctx, query, args...)
}
func (d *txDriver) Tx(context.Context) (dialect.Tx, error) { return nopTx{d.conn}, nil }
func (d *txDriver) BeginTx(context.Context, *entsql.TxOptions) (dialect.Tx, error) {
	return nopTx{d.conn}, nil
}
func (*txDriver) Close() error    { return nil }
func (*txDriver) Dialect() string { return dialect.Postgres }

// nopTx is a transaction that reuses the borrowed connection; commit/rollback
// are no-ops because Bus.WithinTx owns the real transaction boundary.
type nopTx struct {
	entsql.Conn
}

func (nopTx) Commit() error   { return nil }
func (nopTx) Rollback() error { return nil }

// txPublisher serializes a DomainEvent into an Envelope and inserts it onto the
// outbox topic via the tx-bound watermill publisher.
type txPublisher struct {
	wm *watermillsql.Publisher
	tx *sql.Tx
	// client is the ent client bound to tx, so a purge of the outbox rolls back with it.
	client *ent.Client
}

func (p *txPublisher) Publish(ctx context.Context, ev DomainEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	occurred := ev.Project().OccurredAt
	if occurred.IsZero() {
		occurred = time.Now().UTC()
	}
	// The message id is always a fresh ULID (short, fits the outbox uuid column).
	// An event with a natural upstream id supplies a DedupKey instead, which the
	// persist consumer uses as source_id so redelivery dedupes.
	var dedupKey string
	if idv, ok := ev.(identifiable); ok {
		dedupKey = idv.EventID()
	}
	env := Envelope{
		ID:          ulid.Make().String(),
		Name:        ev.EventName(),
		Version:     ev.EventVersion(),
		WorkspaceID: ev.Workspace(),
		OccurredAt:  occurred,
		Data:        data,
		DedupKey:    dedupKey,
	}
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	msg := message.NewMessage(env.ID, body)
	return p.wm.Publish(TopicDomainEvents, msg)
}
