package servicecredentials

import (
	"time"

	"github.com/getsphericon/sphericon/ent"
)

// State is where a Service credential stands at a moment: it works, it was revoked, or its
// expiry has passed.
type State string

const (
	StateActive  State = "active"
	StateRevoked State = "revoked"
	StateExpired State = "expired"
)

// StateAt is the state of c at now: revoked once revoked, expired from its expiry on, active
// otherwise.
func StateAt(c *ent.ServiceCredential, now time.Time) State {
	switch {
	case c.RevokedAt != nil:
		return StateRevoked
	case !active(c, now):
		return StateExpired
	}
	return StateActive
}
