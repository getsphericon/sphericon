package datetrigger

// Occurrence is one occurrence of a date (ADR 0126) an Enrollment or a skipped entry is made for,
// by the date's recurrence: an anniversary year (Yearly), or a once date's one occurrence (Once).
// It is the contract's `{recurrence: yearly, year} | {recurrence: once}`. A Contact is enrolled in an
// Automation once per Occurrence, and a switch between yearly and once starts an occurrence of its
// own. A type switch over it that misses a kind fails the lint (gochecksumtype).
//
//sumtype:decl
type Occurrence interface {
	// stored is the occurrence as a column holds it (StoredOccurrence).
	stored() int
}

// Yearly is the occurrence of a yearly date in an anniversary year: the year of the date the
// offset counts from.
type Yearly struct {
	Year int
}

// Once is the one occurrence of a once date.
type Once struct{}

func (y Yearly) stored() int { return y.Year }

// onceStored is how a column holds Once: no anniversary year is 0.
const onceStored = 0

func (Once) stored() int { return onceStored }

// StoredOccurrence is an Occurrence as an int column holds it, encoded (Stored) and decoded
// (Occurrence) here alone: its anniversary year, or 0 for Once, which no year equals.
type StoredOccurrence int

// Stored is the occurrence as its column holds it.
func Stored(o Occurrence) StoredOccurrence { return StoredOccurrence(o.stored()) }

// Occurrence is the occurrence the column holds.
func (s StoredOccurrence) Occurrence() Occurrence {
	if s == onceStored {
		return Once{}
	}
	return Yearly{Year: int(s)}
}
