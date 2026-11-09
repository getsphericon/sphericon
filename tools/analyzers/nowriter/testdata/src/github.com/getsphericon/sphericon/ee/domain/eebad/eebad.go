package eebad

import "io"

func ReportIssued(w io.Writer) error { return nil } // want `rule 5: ReportIssued takes an io.Writer`
