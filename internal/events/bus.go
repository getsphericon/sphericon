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
	watermillsql "github.com/ThreeDotsLabs/watermill-sql/v2/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/getsphericon/sphericon/ent"
	"github.com/oklog/ulid/v2"
)

// outboxSchema / outboxOffsets are the single source of truth for the
// domain_events topic layout. The producer (WithinTx publisher), the subscribers
// (NewSubscriber), and the DDL (InitSchema) all reference these, so the table the
// producer writes, the table the consumers read, and the table InitSchema creates
// cannot drift apart.
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
	txClient.Use(b.hooks...)

	// watermill-sql forbids AutoInitializeSchema on a tx handle (a CREATE TABLE
	// would implicitly commit). The outbox table is created up front by InitSchema.
	wm, err := watermillsql.NewPublisher(sqlTx, watermillsql.PublisherConfig{
		SchemaAdapter:        outboxSchema,
		AutoInitializeSchema: false,
	}, b.logger)
	if err != nil {
		_ = sqlTx.Rollback()
		return fmt.Errorf("outbox publisher: %w", err)
	}

	if err := fn(txClient, &txPublisher{wm: wm, tx: sqlTx}); err != nil {
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

// InitSchema creates the outbox topic's message and offsets tables and tunes
// their autovacuum. It is idempotent (CREATE TABLE IF NOT EXISTS, ALTER TABLE
// SET) and must run at boot before any producer publishes: the tx publisher
// cannot self-initialize the schema. The outbox tables are created here rather
// than by the Atlas migrations, so their per-table autovacuum settings live here
// too.
//
// The outbox is insert-heavy and pruned in batches (PruneOutbox), so it carries
// a steady stream of dead tuples; the offsets table is a handful of rows updated
// on every ack. Vacuuming both at a low dead-tuple ratio keeps bloat small.
func InitSchema(ctx context.Context, db *sql.DB) error {
	queries := append(outboxSchema.SchemaInitializingQueries(TopicDomainEvents), outboxOffsets.SchemaInitializingQueries(TopicDomainEvents)...)
	queries = append(queries,
		fmt.Sprintf(`ALTER TABLE %s SET (autovacuum_vacuum_scale_factor = 0.01, autovacuum_vacuum_threshold = 1000, autovacuum_analyze_scale_factor = 0.02)`, outboxTable()),
		fmt.Sprintf(`ALTER TABLE %s SET (autovacuum_vacuum_scale_factor = 0, autovacuum_vacuum_threshold = 50, autovacuum_analyze_scale_factor = 0, autovacuum_analyze_threshold = 50)`, offsetsTable()),
	)
	for _, q := range queries {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("init domain-events outbox schema: %w", err)
		}
	}
	return timestamptzCreatedAt(ctx, db)
}

// timestamptzCreatedAt converts the outbox created_at from watermill's plain
// TIMESTAMP (wall time of whichever session wrote it) to timestamptz, so the age
// computations (lag gauge, prune floor) are absolute and independent of session
// TimeZone. The column default CURRENT_TIMESTAMP is timestamptz already, so inserts
// are unchanged. Checked first so a converted table is not locked on every boot.
func timestamptzCreatedAt(ctx context.Context, db *sql.DB) error {
	var typ string
	if err := db.QueryRowContext(ctx,
		`SELECT atttypid::regtype::text FROM pg_attribute WHERE attrelid = $1::regclass AND attname = 'created_at'`,
		outboxTable()).Scan(&typ); err != nil {
		return fmt.Errorf("inspect outbox created_at: %w", err)
	}
	if typ == "timestamp with time zone" {
		return nil
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN created_at TYPE timestamptz`, outboxTable())); err != nil {
		return fmt.Errorf("convert outbox created_at to timestamptz: %w", err)
	}
	return nil
}
