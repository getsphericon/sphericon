//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A Segment is built in the console on a typed Condition (ADR 0181), a Broadcast to it reaches
// only the Contact the Condition names, and once the Condition is edited the next one reaches the
// other.
func TestASegmentBuiltAndEditedInTheConsoleReachesTheContactsItsConditionNames(t *testing.T) {
	t.Parallel()
	w := env.NewWorkspace(t).Ready()
	ann, bob := w.NewRecipient(), w.NewRecipient()
	w.ImportContactNamed(ann, "Ann")
	w.ImportContactNamed(bob, "Bob")

	segment := w.SaveSegment(`{"kind":"group","combinator":"and","entries":[
		{"kind":"attribute","attribute":"first_name","test":{"kind":"text","operator":"eq","value":"Ann"}}]}`)
	w.SendBroadcastTo(segment, Broadcast{Subject: "Only Ann", Body: HTML("Hello Ann")})

	assert.Equal(t, "Only Ann", w.Inbox.Wait(Match{To: ann}).Subject)
	w.Inbox.RequireNone(Match{To: bob, Subject: "Only Ann"})

	w.EditSegment(segment, `{"kind":"group","combinator":"and","entries":[
		{"kind":"attribute","attribute":"first_name","test":{"kind":"presence","operator":"set"}},
		{"kind":"group","combinator":"or","not":true,"entries":[
			{"kind":"attribute","attribute":"first_name","test":{"kind":"text","operator":"beginsWith","value":"An"}}]}]}`)
	w.SendBroadcastTo(segment, Broadcast{Subject: "All but Ann", Body: HTML("Hello")})

	assert.Equal(t, "All but Ann", w.Inbox.Wait(Match{To: bob}).Subject)
	w.Inbox.RequireNone(Match{To: ann, Subject: "All but Ann"})
}
