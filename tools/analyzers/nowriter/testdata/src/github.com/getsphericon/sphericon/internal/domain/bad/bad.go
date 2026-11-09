package bad

import (
	"context"
	"io"
)

func Report(w io.Writer, id string) error { return nil } // want `rule 5: Report takes an io.Writer`

func report(ctx context.Context, w io.Writer) error { return nil } // want `rule 5: report takes an io.Writer`

type Module struct{}

func (*Module) Write(ctx context.Context, w io.Writer) error { return nil } // want `rule 5: Write takes an io.Writer`

func Export(ctx context.Context) (func(w io.Writer) error, error) { // want `rule 5: a function type takes an io.Writer`
	return func(w io.Writer) error { return nil }, nil // want `rule 5: a function literal takes an io.Writer`
}

type Options struct {
	Out io.Writer // want `rule 5: the field Out holds an io.Writer`
	Run func(ctx context.Context, stdout io.Writer) error // want `rule 5: a function type takes an io.Writer`
}

type writer = io.Writer

func Aliased(w writer) {} // want `rule 5: Aliased takes an io.Writer`
