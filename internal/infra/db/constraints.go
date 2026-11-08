package db

import (
	"context"
	"database/sql"
	"errors"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// A Constraint is what the owner of a table declares for one of its unique (or exclusion)
// constraints, by the constraint's exact name (ADR 0106): the refusal a write that breaks it
// becomes, or a fault no client can cause.
type Constraint struct {
	name    string
	refusal error
	reason  string
}

// Refusal declares that a write breaking the constraint named is refused with err, the owner's
// exported sentinel: the subject's state, which a client can reach (a taken key).
func Refusal(constraint string, err error) Constraint {
	return Constraint{name: constraint, refusal: err}
}

// Fault declares that no client can break the constraint named, for the reason given (a random
// token, a key the code derives): a write that breaks it is a fault and its error goes up as it is.
func Fault(constraint, reason string) Constraint {
	return Constraint{name: constraint, reason: reason}
}

// Constraints are the declarations of one owner, made beside the functions that write its tables.
type Constraints struct {
	owner string
	list  []Constraint
}

// Declaration is one Constraint with the package that declared it, as Declarations lists it.
type Declaration struct {
	// Owner is the import path of the declaring package.
	Owner      string
	Constraint string
	// Refusal is the declared refusal, nil for a fault.
	Refusal error
	// Reason is why a fault cannot be caused by a client, empty for a refusal.
	Reason string
}

var (
	declaredMu sync.Mutex
	declared   []Declaration
)

// Declare makes the declarations of the calling package. It is called once per owner, for a
// package-level variable, so the declarations sit beside the writes they guard; it also records
// them, with the package, for the test that holds every declaration to the schema and to the
// ownership of the tables (internal/app).
func Declare(cs ...Constraint) Constraints {
	owner := callerPackage()
	declaredMu.Lock()
	defer declaredMu.Unlock()
	for _, c := range cs {
		declared = append(declared, Declaration{Owner: owner, Constraint: c.name, Refusal: c.refusal, Reason: c.reason})
	}
	return Constraints{owner: owner, list: slices.Clone(cs)}
}

// Declarations lists every declaration the packages of the binary made, in the order made.
func Declarations() []Declaration {
	declaredMu.Lock()
	defer declaredMu.Unlock()
	return slices.Clone(declared)
}

// callerPackage is the import path of the package that called Declare: the function two frames
// up is that package's variable initialisation (or a function of it), named "<path>.<func>".
func callerPackage() string {
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		return ""
	}
	name := runtime.FuncForPC(pc).Name()
	slash := strings.LastIndex(name, "/")
	if dot := strings.Index(name[slash+1:], "."); dot >= 0 {
		return name[:slash+1+dot]
	}
	return name
}

// Savepointer is the handle a write runs on, an *ent.Client or an *ent.Scoped: it runs a function
// under a savepoint of the transaction it is bound to, or as it is when it is bound to none.
type Savepointer interface {
	Savepoint(ctx context.Context, fn func() error) error
}

// Write runs write on the handle it writes through, under a savepoint when that handle is bound to
// a transaction, so a refused write leaves the caller's transaction usable whoever opened it. The
// error of the write is read against the declarations: a unique or exclusion violation of a
// constraint declared a refusal is that refusal; any other error, a fault's or a foreign key's or
// a CHECK's violation included, is returned unchanged.
func (cs Constraints) Write(ctx context.Context, on Savepointer, write func() error) error {
	return cs.read(on.Savepoint(ctx, write))
}

// Inserted reads the error of an insert written ON CONFLICT ... DO NOTHING, the way a write that is
// done when its row is already there says so (ADR 0106): ent reports the insert it skipped as
// sql.ErrNoRows. It reports whether the row was written, and any other error as it is.
func Inserted(err error) (bool, error) {
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	}
	return false, err
}

// read is the declared refusal err stands for, or err.
func (cs Constraints) read(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	if pgErr.Code != pgerrcode.UniqueViolation && pgErr.Code != pgerrcode.ExclusionViolation {
		return err
	}
	for _, c := range cs.list {
		if c.name == pgErr.ConstraintName && c.refusal != nil {
			return c.refusal
		}
	}
	return err
}
