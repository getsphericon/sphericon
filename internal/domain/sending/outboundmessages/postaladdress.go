package outboundmessages

import (
	"context"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ErrPostalAddressRequired refuses to arm marketing email (Schedule or Send of an email Broadcast,
// Publish of an Automation with an email send step) while the Workspace has no postal address,
// which the footer of every marketing email carries (ADR 0156). It is checked where the send is
// armed, never at send time, so nothing stops halfway through a send; a Transactional send and a
// Test send need no address.
var ErrPostalAddressRequired = validation.PostalAddressRequired("outbound: the workspace has no postal address", i18n.ErrorsPostalAddressRequired)

// HasPostalAddress reports whether the Workspace has a postal address: non-empty text after
// trimming (ADR 0156), nothing checked further.
func HasPostalAddress(ws *ent.Workspace) bool {
	return strings.TrimSpace(ws.PostalAddress) != ""
}

// PostalAddressArmable refuses to arm marketing email on ch while the Workspace of s has no postal
// address (ErrPostalAddressRequired). Only email carries the address: the other Channels are never
// refused.
func PostalAddressArmable(ctx context.Context, s *ent.Scoped, ch channels.Channel) error {
	if ch != channels.Email {
		return nil
	}
	ws, err := s.Workspace(ctx)
	if err != nil {
		return err
	}
	if !HasPostalAddress(ws) {
		return ErrPostalAddressRequired
	}
	return nil
}
