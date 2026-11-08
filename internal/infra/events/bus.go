package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/oklog/ulid/v2"

	"github.com/getsphericon/sphericon/ent"
)

// Publisher publishes a typed domain event. The implementation handed to Bus.WithinTx writes in
// the same transaction as the state change: the Event's row and its consumers' jobs, so the event
// is published iff the state commits.
type Publisher interface {
	Publish(_ context.Context, ev DomainEvent) error
}

// Bus is the producer-side entry point. It owns the database pool and opens a transaction per
// WithinTx call that carries the ent writes, the Event rows and the event jobs.
type Bus struct {
	db    *sql.DB
	hooks []ent.Hook
	jobs  JobQueue
	// consumers are the consumers every Publish inserts jobs for (UseConsumers).
	consumers []Consumer
	// stored run for every Event newly stored, once its transaction committed (OnStored).
	stored []func(context.Context, DomainEvent)

	// newTxClient builds the ent client bound to a transaction, with the hooks every such client
	// carries (the wiring's internal/app/entclient.NewTx).
	newTxClient func(dialect.Driver) *ent.Client
	// now is the Instance clock an envelope is stamped by.
	now func() time.Time
}

// New builds a Bus over the application's database pool. newTxClient builds the ent client of
// each transaction the bus opens; the wiring passes the one that carries the application's hooks.
// now is the Instance clock (app.Clock) that dates an envelope: when it is recorded, and when it
// occurred unless the event says.
func New(pool *sql.DB, newTxClient func(dialect.Driver) *ent.Client, now func() time.Time) *Bus {
	return &Bus{db: pool, newTxClient: newTxClient, now: now}
}

// Use registers ent hooks on the transaction client of every later WithinTx, the way
// Client.Use does for a long-lived client. Register at composition time, before the
// bus is shared with concurrent callers; tests use it to inject faults into a
// transaction the bus opens itself.
func (b *Bus) Use(hooks ...ent.Hook) {
	b.hooks = append(b.hooks, hooks...)
}

// OnStored registers what runs for every Event newly stored, once the transaction that stored
// it committed: neither a repeat nor a rolled-back Event reaches it. It runs in the publisher's
// context, uncancelled, and cannot fail the publish, which is already committed. Register at
// composition time, before the bus is shared with concurrent callers.
func (b *Bus) OnStored(fs ...func(context.Context, DomainEvent)) {
	b.stored = append(b.stored, fs...)
}

// WithinTx runs fn inside a single SQL transaction. The *ent.Client handed to fn is bound to
// that transaction, and so is the Publisher: what an event published via pub writes, its Event
// row and its consumers' jobs, joins the transaction. The tx commits iff fn returns nil, so state
// and events commit (or roll back) together.
//
// In tests this rides go-txdb's savepoint nesting, so the commit here is a
// RELEASE SAVEPOINT contained by the per-test rollback.
func (b *Bus) WithinTx(ctx context.Context, fn func(tx *ent.Client, pub Publisher) error) error {
	return b.within(ctx, func(tx *ent.Client, pub *txPublisher) error { return fn(tx, pub) })
}

// Publish publishes ev in a transaction of its own, for a request that changes no state of its
// own and only asks for something to happen: a password reset, a verification link sent again.
// A change of state publishes its event in the transaction of the change instead (WithinTx and
// its scoped variants), so the event exists iff the change commits.
func (b *Bus) Publish(ctx context.Context, ev DomainEvent) error {
	return b.WithinTx(ctx, func(_ *ent.Client, pub Publisher) error { return pub.Publish(ctx, ev) })
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

	// ent client bound to the *sql.Tx the event jobs are inserted on.
	txClient := b.newTxClient(newTxDriver(sqlTx))
	txClient.Use(b.hooks...)

	pub := &txPublisher{tx: sqlTx, client: txClient, jobs: b.jobs, consumers: b.consumers, stored: b.stored, now: b.now}
	if err := fn(txClient, pub); err != nil {
		_ = sqlTx.Rollback()
		return err
	}
	if err := sqlTx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	for _, f := range pub.committed {
		f()
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
// transaction, as internal/domain/identity/workspacefeed needs.
func (d *txDriver) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	// raw sql: forwards the caller's statement to the transaction unchanged; it adds none of its own.
	return d.conn.ExecContext(ctx, query, args...)
}
func (d *txDriver) Tx(context.Context) (dialect.Tx, error) { return nopTx{d.conn}, nil }
func (d *txDriver) BeginTx(context.Context, *entsql.TxOptions) (dialect.Tx, error) {
	return nopTx{d.conn}, nil
}

// TxBound marks the driver as bound to the bus's transaction (ent.TxBound), so a write that may be
// refused runs under a savepoint of it (ent.Client.Savepoint, ADR 0106).
func (*txDriver) TxBound() {}

func (*txDriver) Close() error    { return nil }
func (*txDriver) Dialect() string { return dialect.Postgres }

// nopTx is a transaction that reuses the borrowed connection; commit/rollback
// are no-ops because Bus.WithinTx owns the real transaction boundary.
type nopTx struct {
	entsql.Conn
}

func (nopTx) Commit() error   { return nil }
func (nopTx) Rollback() error { return nil }

// txPublisher stamps a DomainEvent's Envelope and writes what publishing it means, all on tx.
type txPublisher struct {
	tx *sql.Tx
	// client is the ent client bound to tx, which the Event rows are written with.
	client *ent.Client
	// jobs is the bus's job queue (nil when it was given none); the event jobs are inserted and
	// a purge of the queue runs on tx.
	jobs JobQueue
	// consumers are the bus's consumers, whose jobs Publish inserts on tx.
	consumers []Consumer
	// stored are the bus's OnStored functions, run for a newly stored Event after the commit.
	stored []func(context.Context, DomainEvent)
	// committed runs once tx has committed, in order.
	committed []func()
	// now is the bus's clock.
	now func() time.Time
}

// Publish stamps the envelope of ev and, for an event that is an Event (Projected), writes its
// row in the transaction. An event whose row already exists is a repeat, and is not published
// again: no consumer sees it twice. Otherwise it inserts, in the transaction, the job of every
// consumer that wants the event. An error leaves the caller to roll back, so no state commits
// without its event.
func (p *txPublisher) Publish(ctx context.Context, ev DomainEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	recorded := p.now().UTC()
	projection, projected := ProjectionOf(ev)
	occurred := projection.OccurredAt
	if occurred.IsZero() {
		occurred = recorded
	}
	// The envelope id is always a fresh ULID. An event with a natural upstream id supplies a
	// DedupKey besides, which its Event row is stored under (source_id), so a redelivery is
	// told from a new event.
	var dedupKey string
	if idv, ok := ev.(identifiable); ok {
		dedupKey = idv.EventID()
	}
	env := Envelope{
		ID:          ulid.Make().String(),
		Name:        ev.EventName(),
		Version:     ev.EventVersion(),
		WorkspaceID: WorkspaceOf(ev),
		OccurredAt:  occurred,
		RecordedAt:  recorded,
		Data:        data,
		DedupKey:    dedupKey,
		CausedBy:    causeOf(ctx),
	}
	stamp(ctx, &env)
	if projected {
		first, err := storeEvent(ctx, p.client, env, ev, projection)
		if err != nil {
			return err
		}
		if !first {
			return nil // a repeat: published when it first arrived
		}
		for _, f := range p.stored {
			p.committed = append(p.committed, func() { f(context.WithoutCancel(ctx), ev) })
		}
	}
	if err := p.queueConsumers(ctx, env, ev); err != nil {
		return fmt.Errorf("queue the consumers of %q: %w", env.Name, err)
	}
	return nil
}
