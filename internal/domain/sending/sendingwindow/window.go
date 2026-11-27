package sendingwindow

import (
	"slices"
	"time"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
)

// week is how far ahead the window is looked for: a window open at no moment of a week in every
// zone at once never is, and the message has no local time (ADR 0134).
const week = 7 * 24 * time.Hour

// Window is a Sending window: the hours of the days of the week it is open, in the local time of
// whichever zone it is read in. A day it has no hours for stays closed.
type Window struct {
	days map[localcalendar.Weekday]workspaces.OpenHours
}

// NewWindow is the window open on the hours given, one stretch per day at most, as the
// Workspace's settings keep them.
func NewWindow(hours []workspaces.OpenHours) Window {
	days := make(map[localcalendar.Weekday]workspaces.OpenHours, len(hours))
	for _, h := range hours {
		days[h.Day] = h
	}
	return Window{days: days}
}

// openIn reports whether the window is open at t in loc: t reads there as a minute inside its
// day's hours.
func (w Window) openIn(t time.Time, loc *time.Location) bool {
	local := t.In(loc)
	h, ok := w.days[localcalendar.DateOf(local).Weekday()]
	minute := local.Hour()*60 + local.Minute()
	return ok && h.OpensAt <= minute && minute < h.ClosesAt
}

// openInAll reports whether the window is open at t in every one of zones.
func (w Window) openInAll(t time.Time, zones []*time.Location) bool {
	for _, loc := range zones {
		if !w.openIn(t, loc) {
			return false
		}
	}
	return true
}

// Opens is the first moment from at on at which the window is open in every one of zones, and
// whether one comes within a week. Where the window is open in every zone at once, that stretch
// begins when the last of them opens: a moment one zone's hours open, or one its clock changes at,
// so those are the moments looked at, in order. With no zone it is open at once.
func (w Window) Opens(at time.Time, zones []*time.Location) (time.Time, bool) {
	if w.openInAll(at, zones) {
		return at, true
	}
	until := at.Add(week)
	var candidates []time.Time
	for _, loc := range zones {
		candidates = append(candidates, w.starts(at, until, loc)...)
	}
	slices.SortFunc(candidates, func(a, b time.Time) int { return a.Compare(b) })
	for _, c := range candidates {
		if w.openInAll(c, zones) {
			return c, true
		}
	}
	return time.Time{}, false
}

// starts is every moment after at and up to until at which a stretch of the window may begin in
// loc: each day's opening there, and each change of its clock, which can bring a minute inside the
// day's hours back (a fall-back change) or move past one (a spring-forward change).
func (w Window) starts(at, until time.Time, loc *time.Location) []time.Time {
	var out []time.Time
	keep := func(t time.Time) {
		if t.After(at) && !t.After(until) {
			out = append(out, t)
		}
	}
	first := localcalendar.DateOf(at.In(loc)).AddDays(-1)
	for i := range 10 {
		day := first.AddDays(i)
		if h, ok := w.days[day.Weekday()]; ok {
			keep(day.At(h.OpensAt, loc))
		}
	}
	for t := at; ; {
		_, end := t.In(loc).ZoneBounds()
		if end.IsZero() || end.After(until) {
			break
		}
		keep(end)
		t = end
	}
	return out
}
