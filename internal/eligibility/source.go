package eligibility

import (
	"fmt"
	"strconv"
	"strings"
)

// Channel identifiers. Email is the only built channel; sms is reserved.
const ChannelEmail = "email"

// Sending sources — the unit a destination unsubscribes from. A source is
// automatic, never hand-authored: it IS the sender. All broadcasts share one
// scope; each automation is its own; "everything" is the reserved deliberate
// "leave entirely" opt-out (only ever an unsubscribe scope, never a send source).
const (
	SourceBroadcasts = "broadcasts"
	SourceEverything = "everything"
)

// AutomationSource is the per-automation unsubscribe scope.
func AutomationSource(automationID int64) string {
	return fmt.Sprintf("automation:%d", automationID)
}

// ParseAutomationSource is the inverse of AutomationSource: it returns the
// automation id when src is an "automation:<id>" scope, and false otherwise.
func ParseAutomationSource(src string) (int64, bool) {
	rest, ok := strings.CutPrefix(src, "automation:")
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
