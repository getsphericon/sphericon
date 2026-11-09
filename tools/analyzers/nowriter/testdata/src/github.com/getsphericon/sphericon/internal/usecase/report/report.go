package report

import (
	"context"
	"io"
)

type UseCase struct {
	out io.Writer // want `rule 5: the field out holds an io.Writer`
}

func (*UseCase) Report(ctx context.Context, w io.Writer) error { return nil } // want `rule 5: Report takes an io.Writer`

func write(w io.Writer) {} // want `rule 5: write takes an io.Writer`
