package bad

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
)

type UseCase struct{}

func New() *UseCase { return &UseCase{} }

func Operation(context.Context) error { return nil } // want `rule 5: Operation is an exported function of a use case package`

func helper(context.Context) error { return nil }

func (*UseCase) Tuple(context.Context, *ent.Scoped) (int, []string, error) { return 0, nil, nil } // want `rule 5: Tuple returns 3 values`

func (*UseCase) Pair(context.Context, *ent.Scoped) (int, string) { return 0, "" } // want `rule 5: Pair returns a second value that is not error`

func (*UseCase) Named(context.Context, *ent.Scoped) (verified bool, err error) { return false, nil } // want `rule 5: Named has named results`

func (*UseCase) Each(context.Context, *ent.Scoped, []string) []error { return nil } // want `rule 5: Each returns \[\]error`

func (*UseCase) Three(context.Context, *ent.Scoped, int64, string, string) error { return nil } // want `rule 5: Three takes 3 arguments after ctx and the scope`

func (*UseCase) RawThree(context.Context, *ent.Client, int64, string, string) error { return nil } // want `rule 5: RawThree takes 3 arguments after ctx and the scope`

func (*UseCase) Unscoped(context.Context, int64, string, string) error { return nil } // want `rule 5: Unscoped takes 3 arguments after ctx and the scope`

func (*UseCase) At(_ context.Context, _ *ent.Scoped, now time.Time) error { return nil } // want `rule 5: At takes now`

func (*UseCase) Clocked(context.Context, func() time.Time) error { return nil } // want `rule 5: Clocked takes a clock`

func (*UseCase) Stamp(context.Context) time.Time {
	return time.Now() // want `rule 5: the use case reads time.Now`
}

var clock = time.Now // want `rule 5: the use case reads time.Now`

func (*UseCase) unexported(context.Context) (int, int, error) { return 0, 0, nil }
