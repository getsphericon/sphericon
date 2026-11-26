package outboundmessages

import (
	"fmt"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
)

// recipientLimits is what the limits of one recipient make of a message of a kind (ADR 0178):
// whether it counts toward its Destinations' Frequency caps, and whether the Frequency cap and the
// Sending window may make it wait.
type recipientLimits struct {
	// counts: the message takes a place under the Frequency cap of each Destination it is claimed for.
	counts bool
	// waits: the Frequency cap and the Sending window hold the message back.
	waits bool
}

// recipientLimitsOf is the rule of a message of kind. Its switch has no default, so a new kind of
// Outbound message fails the exhaustive linter until it says whether it counts and waits. A Test send
// is no Outbound message (it never reaches Send) and neither counts nor waits.
func recipientLimitsOf(kind outboundmessage.Kind) (recipientLimits, error) {
	switch kind {
	case outboundmessage.KindBroadcast, outboundmessage.KindAutomation:
		// Marketing: every Broadcast's and Automation's message counts and may wait.
		return recipientLimits{counts: true, waits: true}, nil
	case outboundmessage.KindTransactional, outboundmessage.KindConfirmation:
		// A message about the person's own business (Transactional, email-only, ADR 0077) or address
		// (a confirmation email, ADR 0013) neither counts nor waits.
		return recipientLimits{}, nil
	}
	return recipientLimits{}, fmt.Errorf("outbound: no recipient limits for kind %q", kind)
}
