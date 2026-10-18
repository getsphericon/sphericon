package testhelper

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"net/url"
	"strings"

	"github.com/getsphericon/sphericon/config"
	"github.com/jackc/pgx/v5"
)

// maxWorkers bounds how many test processes run against one checkout's server at once;
// go test runs as many as GOMAXPROCS.
const maxWorkers = 64

// leases holds this process's lease connections for its lifetime: referenced, so the
// garbage collector never closes one. A session advisory lock is released by the server
// when the connection drops, so a crashed or finished process frees its worker database
// with no cleanup hook (a test package has no TestMain).
//
// It is a bare pgx connection, not a database/sql pool: a pool keeps a goroutine alive,
// and the first test of a package may well call Setup inside a testing/synctest bubble,
// which must end with no goroutine of its own left behind.
var leases []*pgx.Conn

// leaseWorkerDatabase gives this test process a database of its own at the fixture
// state, so test packages run in parallel without sharing rows or locks. It returns that
// database's URL.
//
// The databases are `<test db>_tpl_<hash>`, the template, and `<test db>_w<i>`, the
// workers. The template holds the schema only and is built once per hash of what decides
// it (schemaHash), under an advisory lock, by whichever process comes first. A worker is
// claimed by taking the advisory lock of the first free index, and is cloned from the
// template when it is missing or was cloned from an older one. The caller loads the
// fixtures into it (loadFixtures), which also wipes whatever a previous process left.
func leaseWorkerDatabase(ctx context.Context, cfg *config.Config, projectRoot string) (string, error) {
	base, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		return "", fmt.Errorf("parse database url: %w", err)
	}
	baseName := strings.TrimPrefix(base.Path, "/")

	hash, err := schemaHash()
	if err != nil {
		return "", err
	}
	template := baseName + "_tpl_" + hash[:12]
	if err := ensureTemplate(ctx, cfg, base, baseName, template, projectRoot); err != nil {
		return "", err
	}

	// The shared test database is the place the locks live: advisory locks are per database.
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return "", fmt.Errorf("connect to the test database: %w", err)
	}
	worker, err := claimWorker(ctx, conn, baseName)
	if err == nil {
		err = cloneWorker(ctx, conn, worker, template)
	}
	if err != nil {
		_ = conn.Close(ctx)
		return "", err
	}
	leases = append(leases, conn) // kept open on purpose: closing it would release the lease
	return withDatabase(base, worker), nil
}

// ensureTemplate builds the template database if this hash has none yet.
func ensureTemplate(ctx context.Context, cfg *config.Config, base *url.URL, baseName, template, projectRoot string) error {
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to the test database: %w", err)
	}
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	key := lockKey("template:" + template)
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
		return err
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, key) }()

	if databaseExists(ctx, conn, template) {
		return nil
	}

	// Built under another name and renamed when complete, so a template that exists is
	// always whole, even after a build that was interrupted.
	building := template + "_build"
	if _, err := conn.Exec(ctx, `DROP DATABASE IF EXISTS `+ident(building)+` WITH (FORCE)`); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, `CREATE DATABASE `+ident(building)); err != nil {
		return err
	}
	built := *cfg
	built.DatabaseURL = withDatabase(base, building)
	buildDB, err := sql.Open("pgx", built.DatabaseURL)
	if err != nil {
		return err
	}
	migrateErr := migrateSchema(ctx, buildDB, &built)
	_ = buildDB.Close()
	if migrateErr != nil {
		_, _ = conn.Exec(ctx, `DROP DATABASE IF EXISTS `+ident(building)+` WITH (FORCE)`)
		return fmt.Errorf("build the test template: %w", migrateErr)
	}
	if _, err := conn.Exec(ctx, `ALTER DATABASE `+ident(building)+` RENAME TO `+ident(template)); err != nil {
		return err
	}

	// Templates of older hashes are dead weight; a failure to drop one (a worker is
	// still being cloned from it) leaves it for the next build.
	like := strings.NewReplacer(`\`, `\\`, `_`, `\_`, `%`, `\%`).Replace(baseName) + `\_tpl\_%`
	rows, err := conn.Query(ctx, `SELECT datname FROM pg_database WHERE datname LIKE $1 AND datname <> $2`, like, template)
	if err != nil {
		return nil
	}
	stale, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil
	}
	for _, name := range stale {
		_, _ = conn.Exec(ctx, `DROP DATABASE IF EXISTS `+ident(name))
	}
	return nil
}

// claimWorker takes the lowest free worker index and returns that worker's database name.
func claimWorker(ctx context.Context, conn *pgx.Conn, baseName string) (string, error) {
	for i := range maxWorkers {
		name := fmt.Sprintf("%s_w%d", baseName, i)
		var got bool
		if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockKey("worker:"+name)).Scan(&got); err != nil {
			return "", err
		}
		if got {
			return name, nil
		}
	}
	return "", fmt.Errorf("all %d test worker databases are taken", maxWorkers)
}

// cloneWorker makes the worker database a copy of the template unless it already is one.
// Which template a worker came from is kept in the database's comment.
func cloneWorker(ctx context.Context, conn *pgx.Conn, worker, template string) error {
	var from *string
	err := conn.QueryRow(ctx,
		`SELECT shobj_description(oid, 'pg_database') FROM pg_database WHERE datname = $1`, worker).Scan(&from)
	if err == nil && from != nil && *from == template {
		return nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if _, err := conn.Exec(ctx, `DROP DATABASE IF EXISTS `+ident(worker)+` WITH (FORCE)`); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, `CREATE DATABASE `+ident(worker)+` TEMPLATE `+ident(template)); err != nil {
		return fmt.Errorf("clone %s: %w", worker, err)
	}
	_, err = conn.Exec(ctx, `COMMENT ON DATABASE `+ident(worker)+` IS `+literal(template))
	return err
}

func databaseExists(ctx context.Context, conn *pgx.Conn, name string) bool {
	var one int
	return conn.QueryRow(ctx, `SELECT 1 FROM pg_database WHERE datname = $1`, name).Scan(&one) == nil
}

// withDatabase is the server URL pointed at another database.
func withDatabase(server *url.URL, name string) string {
	u := *server
	u.Path = "/" + name
	return u.String()
}

func ident(name string) string { return pgx.Identifier{name}.Sanitize() }

func literal(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func lockKey(s string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return int64(h.Sum64())
}

// schemaHash digests everything that decides the schema a template holds.
func schemaHash() (string, error) {
	return hashTrees(projectRoot(), "ent/schema", "internal/events/bus.go", "go.mod")
}
