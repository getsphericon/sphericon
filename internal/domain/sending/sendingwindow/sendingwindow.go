// Package sendingwindow is the Sending window of the sending context (ADR 0134): for a number, the
// Contact's time zone and a moment, whether the Workspace's window is open in every zone that may be
// the recipient's, and if not the next moment it is open in all of them, or that none comes within
// a week. The zones are those the number's prefix may be in (phone.Zones: every zone of its country
// when the prefix maps none), and the Contact's own when it is a valid one, which only narrows the
// window. It writes nothing: the windows are the Workspace's settings
// (internal/domain/identity/workspaces), which it reads.
package sendingwindow

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/getsphericon/sphericon/ent"
	entwindow "github.com/getsphericon/sphericon/ent/sendingwindow"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
	"github.com/getsphericon/sphericon/internal/kernel/reportday"
)

// Module is the Sending window module. It is stateless.
type Module struct{}

// New builds the module.
func New() *Module { return &Module{} }

// Rule is the Sending window rule of a Workspace's Channel: which of its windows holds a number. A
// country's row outranks the Channel's for a number of that country (workspaces.TakesCountry).
type Rule struct {
	channel channels.Channel
	// own is the Channel's own window, nil without one; countries the windows of countries.
	own       *Window
	countries map[string]Window
}

// Rule reads the Sending window rule of the scoped Workspace's Channel ch from its window rows. A
// Channel no window holds (workspaces.HasSendingWindow) has a rule with none.
func (m *Module) Rule(ctx context.Context, s *ent.Scoped, ch channels.Channel) (Rule, error) {
	rule := Rule{channel: ch, countries: map[string]Window{}}
	if !workspaces.HasSendingWindow(ch) {
		return rule, nil
	}
	rows, err := s.SendingWindow().Query().Where(entwindow.ChannelEQ(ch)).All(ctx)
	if err != nil {
		return Rule{}, fmt.Errorf("sendingwindow: read the windows: %w", err)
	}
	for _, r := range rows {
		w := NewWindow(workspaces.WindowHours(r))
		if r.Country == nil {
			rule.own = &w
			continue
		}
		rule.countries[*r.Country] = w
	}
	return rule, nil
}

// window is the window that holds number, and whether one does: its country's row when the
// Channel takes one and the Workspace has it, else the Channel's own.
func (r Rule) window(number string) (Window, bool) {
	if workspaces.TakesCountry(r.channel) {
		if w, ok := r.countries[phone.Country(number)]; ok {
			return w, true
		}
	}
	if r.own == nil {
		return Window{}, false
	}
	return *r.own, true
}

// Judgement is what the Sending window says of a message to a number at a moment.
type Judgement struct {
	// Windowed is whether a window holds the number. Without one the message may go at any moment:
	// Open is true and the rest is zero.
	Windowed bool
	// Zones is how many zones the window was read in: the number's, with the Contact's own when it
	// is valid and another.
	Zones int
	// Open is whether the window is open at the moment in every zone.
	Open bool
	// Opens is, when it is not open, the next moment it is open in every zone; zero when none comes
	// within a week.
	Opens time.Time
}

// NoLocalTime reports whether no moment of the week is open in every zone: the message is Skipped
// rather than sent at night or kept waiting for ever (ADR 0134).
func (j Judgement) NoLocalTime() bool { return !j.Open && j.Opens.IsZero() }

// SingleZone reports whether a window judged the message by one zone alone.
func (j Judgement) SingleZone() bool { return j.Windowed && j.Zones == 1 }

// Judge reads the window that holds number at the moment at, in every zone the recipient may be in:
// those of the number (phone.Zones), and contactZone, the Contact's time zone, when it is a valid
// IANA zone. A Contact's zone only narrows the window: a number in no zone at all (a global service
// number) has no local time whatever the Contact's zone says. A zone the Instance's zone database
// cannot load is an error, never a zone left out.
func (r Rule) Judge(number string, contactZone *string, at time.Time) (Judgement, error) {
	w, ok := r.window(number)
	if !ok {
		return Judgement{Open: true}, nil
	}
	names := phone.Zones(number)
	if len(names) == 0 {
		return Judgement{Windowed: true}, nil
	}
	locs := make([]*time.Location, 0, len(names)+1)
	for _, name := range names {
		loc, err := reportday.Load(name)
		if err != nil {
			return Judgement{}, fmt.Errorf("sendingwindow: a zone of %s: %w", number, err)
		}
		locs = append(locs, loc)
	}
	if own, ok := contactLocation(contactZone); ok && !slices.Contains(names, own.String()) {
		locs = append(locs, own)
	}
	opens, ok := w.Opens(at, locs)
	j := Judgement{Windowed: true, Zones: len(locs), Open: ok && opens.Equal(at)}
	if ok && !j.Open {
		j.Opens = opens
	}
	return j, nil
}

// contactLocation is the zone a Contact's time zone names, when the zone database loads it: one it
// does not (never set, or a name it no longer knows) counts for nothing.
func contactLocation(name *string) (*time.Location, bool) {
	if name == nil {
		return nil, false
	}
	loc, err := reportday.Load(*name)
	return loc, err == nil
}
