// Package skipreason names why an Outbound send is Skipped (GLOSSARY: Outcome, ADR 0178): the one
// closed list of reasons a send is final for its Destination, which the Outbound message stores and
// the contracts' SkippedReason carries. It is a leaf, so the ent schema types its column with it.
// Send-eligibility gives the first four; Outbound send the rest.
package skipreason

import "slices"

// Reason is why a send is Skipped.
type Reason string

// The reasons Send-eligibility gives a Destination (ADR 0001, 0013, 0077).
const (
	// Suppressed: the Destination is on the Workspace's Suppression list.
	Suppressed Reason = "suppressed"
	// UnsubscribedEverything: the Destination unsubscribed from everything on the Channel.
	UnsubscribedEverything Reason = "unsubscribed_everything"
	// UnsubscribedSource: the Destination unsubscribed from the message's Sending source.
	UnsubscribedSource Reason = "unsubscribed_source"
	// Unconfirmed: the Channel asks for a Confirmation the Destination does not have.
	Unconfirmed Reason = "unconfirmed"
)

// The reasons Outbound send gives a send.
const (
	// NoDestination: the Contact has no Destination on the Channel (no address, no Binding), so
	// there is nothing to send to (ADR 0081).
	NoDestination Reason = "no_destination"
	// ContactErased: the send's Contact was erased after it was queued (ADR 0021); nothing is
	// recorded for it.
	ContactErased Reason = "contact_erased"
	// NoLocalTime: no moment of the week is open in every zone that may be the recipient's, so a
	// marketing SMS cannot wait for its Sending window (ADR 0134).
	NoLocalTime Reason = "no_local_time"
	// FrequencyCap: the Destination is over its Frequency cap (ADR 0137, 0178).
	FrequencyCap Reason = "frequency_cap"
)

// all is every reason, Send-eligibility's first.
var all = []Reason{
	Suppressed, UnsubscribedEverything, UnsubscribedSource, Unconfirmed,
	NoDestination, ContactErased, NoLocalTime, FrequencyCap,
}

// All is every reason, in the order of the closed list.
func All() []Reason { return slices.Clone(all) }

// Values is every reason, as an ent enum column takes them.
func (Reason) Values() []string {
	values := make([]string, len(all))
	for i, r := range all {
		values[i] = string(r)
	}
	return values
}

// Count is how many sends were Skipped for one reason.
type Count struct {
	Reason Reason
	Count  int
}

// Counts lists the count by holds of every reason, in the order of the closed list: a reason by
// lacks reads zero, so a report names every reason whatever its rows hold.
func Counts(by map[Reason]int) []Count {
	counts := make([]Count, len(all))
	for i, r := range all {
		counts[i] = Count{Reason: r, Count: by[r]}
	}
	return counts
}
