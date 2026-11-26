package datetrigger

import (
	"database/sql/driver"
	"errors"
	"fmt"
)

// Occurrence is one occurrence of a date (ADR 0126) an Enrollment or a skipped entry is made for,
// by the date's recurrence: an anniversary year (Yearly), or a once date's one occurrence (Once).
// It is the contract's `{recurrence: yearly, year} | {recurrence: once}`. A Contact is enrolled in an
// Automation once per Occurrence, and a switch between yearly and once starts an occurrence of its
// own. A type switch over it that misses a kind fails the lint (gochecksumtype).
//
//sumtype:decl
type Occurrence interface {
	// stored is the occurrence as a column holds it (StoredOccurrence).
	stored() int64
}

// Yearly is the occurrence of a yearly date in an anniversary year: the year of the date the
// offset counts from.
type Yearly struct {
	Year int
}

// Once is the one occurrence of a once date.
type Once struct{}

func (y Yearly) stored() int64 { return int64(y.Year) }

// onceStored is how a column holds Once: no anniversary year is 0.
const onceStored = 0

func (Once) stored() int64 { return onceStored }

// StoredOccurrence is an Occurrence as an int column holds it, encoded and decoded here alone: its
// anniversary year, or 0 for Once, which no year equals.
type StoredOccurrence struct {
	Occurrence Occurrence
}

// Value is the column's int.
func (s StoredOccurrence) Value() (driver.Value, error) {
	if s.Occurrence == nil {
		return nil, errors.New("datetrigger: an occurrence of no kind")
	}
	return s.Occurrence.stored(), nil
}

// Scan reads the column's int.
func (s *StoredOccurrence) Scan(src any) error {
	n, ok := src.(int64)
	if !ok {
		return fmt.Errorf("datetrigger: an occurrence stored as %T", src)
	}
	if n == onceStored {
		s.Occurrence = Once{}
		return nil
	}
	s.Occurrence = Yearly{Year: int(n)}
	return nil
}

// Stored is the occurrence as its column holds it.
func Stored(o Occurrence) StoredOccurrence { return StoredOccurrence{Occurrence: o} }
