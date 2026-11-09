package eereport

import "io"

type UseCase struct{}

func (*UseCase) Report(w io.Writer) error { return nil } // want `rule 5: Report takes an io.Writer`
