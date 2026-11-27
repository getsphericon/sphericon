package broadcasts

import (
	"context"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// windowPage is how many of an audience's Contacts the share outside the Sending window reads at a
// time, so a large audience is never held in memory whole.
const windowPage = 1000

// WindowInput is the audience of a Broadcast being written and the moment it would be scheduled
// at: the Contacts Condition matches (nil: every Contact) that a Broadcast on Channel, routed
// through the Integration IntegrationID names (nil: the Channel's default), would take, as the
// audience count of ADR 0149 counts them.
type WindowInput struct {
	Condition     *conditiontree.Segment
	Channel       channels.Channel
	IntegrationID *int64
	At            time.Time
}

// OutsideWindow is the share of an audience the Sending window would hold at a moment (ADR 0134).
type OutsideWindow struct {
	// Held is how many of the audience would wait for their window to open.
	Held int
	// ReachedAt is when the last of them is reached: the latest moment their windows open. Nil when
	// none is held.
	ReachedAt *time.Time
}

// OutsideWindow says how many of the audience the input names the Sending window of its Channel
// would hold if the Broadcast were sent at the input's moment, and when the last of them is
// reached, each judged as Outbound send judges a text: by every zone its number may be in and the
// Contact's own time zone. A Recipient whose zones share no open moment is Skipped (no_local_time),
// not held, so it counts in neither. A Channel no window holds (email, Telegram) holds none. A
// Condition that does not compile is ErrAudienceUncountable.
func (m *Module) OutsideWindow(ctx context.Context, s *ent.Scoped, in WindowInput) (OutsideWindow, error) {
	var out OutsideWindow
	if !workspaces.HasSendingWindow(in.Channel) {
		return out, nil
	}
	audience, err := m.audiences.matching(ctx, s, in.Channel, in.IntegrationID, in.Condition)
	if errors.Is(err, ErrNoAudience) {
		return OutsideWindow{}, ErrAudienceUncountable.With(err.Error())
	}
	if err != nil {
		return OutsideWindow{}, err
	}
	rule, err := m.windows.Rule(ctx, s, in.Channel)
	if err != nil {
		return OutsideWindow{}, err
	}
	var after int64
	for {
		page, err := audience.Clone().
			Where(contact.IDGT(after), contact.PhoneNotNil()).
			Order(ent.Asc(contact.FieldID)).
			Limit(windowPage).
			Select(contact.FieldID, contact.FieldPhone, contact.FieldTimeZone).
			All(ctx)
		if err != nil {
			return OutsideWindow{}, err
		}
		for _, c := range page {
			j, err := rule.Judge(*c.Phone, c.TimeZone, in.At)
			if err != nil {
				return OutsideWindow{}, err
			}
			if j.Open || j.NoLocalTime {
				continue
			}
			out.Held++
			if out.ReachedAt == nil || j.Opens.After(*out.ReachedAt) {
				opens := j.Opens
				out.ReachedAt = &opens
			}
		}
		if len(page) < windowPage {
			return out, nil
		}
		after = page[len(page)-1].ID
	}
}
