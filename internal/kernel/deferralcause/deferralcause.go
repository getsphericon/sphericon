// Package deferralcause names what a Deferral waits for (GLOSSARY: Deferral, ADR 0178): the one
// closed list of causes, which the contracts' DeferralCause carries. It is a leaf, so an ent schema
// can type a column with it. The first three are per source, the last two per recipient.
package deferralcause

// Cause is what a Deferral waits for, and what paces the rest of a sending Broadcast. Each surface
// maps it to its own enum once, in its converter.
type Cause string

const (
	// ProviderLimit: the Integration's Provider limit (ADR 0023).
	ProviderLimit Cause = "provider_limit"
	// SendCap: the Workspace's Send cap (ADR 0067).
	SendCap Cause = "send_cap"
	// SlowDown: the provider asked to slow down, answering "too fast" or "quota exceeded"; only a
	// Deferral names it.
	SlowDown Cause = "provider"
	// SendingWindow: a marketing SMS outside the recipient's Sending window (ADR 0134).
	SendingWindow Cause = "sending_window"
	// FrequencyCap: an Automation's send over its Destination's Frequency cap (ADR 0137, 0178).
	FrequencyCap Cause = "frequency_cap"
)

// all is every cause, the per-source ones first.
var all = []Cause{ProviderLimit, SendCap, SlowDown, SendingWindow, FrequencyCap}

// Values is every cause, as an ent enum column takes them.
func (Cause) Values() []string {
	values := make([]string, len(all))
	for i, c := range all {
		values[i] = string(c)
	}
	return values
}

// Count is how many sends wait for one cause.
type Count struct {
	Cause Cause
	Count int
}

// Counts lists the count by holds of every cause, in the order of the closed list: a cause by
// lacks reads zero, so a report names every cause whatever its rows hold.
func Counts(by map[Cause]int) []Count {
	counts := make([]Count, len(all))
	for i, c := range all {
		counts[i] = Count{Cause: c, Count: by[c]}
	}
	return counts
}
