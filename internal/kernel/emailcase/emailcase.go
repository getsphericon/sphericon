// Package emailcase keeps the Email case (GLOSSARY "Email case"): every stored address is trimmed
// and lower-cased, so one address is one row whatever case it was typed in, and it matches the same
// address wherever it is stored. Every owner of an address column calls Of on each write and on each
// lookup by address (ADR 0107); the column's lower-case CHECK fails a write that skipped it.
package emailcase

import "strings"

// Of is the address in its Email case: without surrounding spaces, lower-cased.
func Of(address string) string { return strings.ToLower(strings.TrimSpace(address)) }

// OfPtr is Of for an optional address: nil stays nil.
func OfPtr(address *string) *string {
	if address == nil {
		return nil
	}
	return new(Of(*address))
}
