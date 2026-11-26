package workspaces

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/frequencycap"
	"github.com/getsphericon/sphericon/ent/sendingwindow"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The unique constraints of the sending rules (ADR 0106): no client can break them.
var _ = db.Declare(
	db.Fault("frequency_caps_workspace_channel_country",
		"ReplaceSendingRules refuses two caps for one Channel and country (ErrSendingRuleRepeated) and deletes the old rows first; Creation.CreateIn writes one per Channel of a new Workspace"),
	db.Fault("sending_windows_workspace_channel_country",
		"ReplaceSendingRules refuses two windows for one Channel and country (ErrSendingRuleRepeated) and deletes the old rows first; Creation.CreateIn writes one per Channel of a new Workspace"),
)

// The Diff keys of a `workspace.sending_rules_change` Audit entry: each list that changed, from
// what it was to what it became.
const (
	DiffFrequencyCaps  = "frequency_caps"
	DiffSendingWindows = "sending_windows"
)

// SendingRules are the Workspace's sending rules (ADR 0134, ADR 0137): its Frequency caps and its
// Sending windows, each in the order Channel then country (the Channel's own row first), with the
// Workspace's revision they were read at (ADR 0119).
type SendingRules struct {
	FrequencyCaps  []FrequencyCap
	SendingWindows []SendingWindow
	Revision       int64
}

// FrequencyCap is one of the Workspace's Frequency caps (ADR 0137): at most MaxMessages marketing
// messages to one Destination on Channel per rolling PeriodMinutes, for the numbers of Country or,
// with none, for every Destination on the Channel. A cap with no maximum (and so no period) removes
// the cap there.
type FrequencyCap struct {
	Channel       channels.Channel `json:"channel"`
	Country       *string          `json:"country"`
	MaxMessages   *int             `json:"maxMessages"`
	PeriodMinutes *int             `json:"periodMinutes"`
}

// SendingWindow is one of the Workspace's Sending windows (ADR 0134): the hours of the days it
// lists, in the recipient's local time, in which a marketing message on Channel may go out, for
// the numbers of Country or, with none, for every number on the Channel. A day it does not list
// stays closed.
type SendingWindow struct {
	Channel channels.Channel `json:"channel"`
	Country *string          `json:"country"`
	Hours   []OpenHours      `json:"hours"`
}

// OpenHours is the stretch of one day a Sending window is open: from the minute of the day OpensAt
// to the later ClosesAt (1440 is midnight at the day's end).
type OpenHours struct {
	Day      localcalendar.Weekday `json:"day"`
	OpensAt  int                   `json:"opensAt"`
	ClosesAt int                   `json:"closesAt"`
}

// ReplaceSendingRulesInput is the whole of the Workspace's sending rules, made on Revision (ADR
// 0119): a cap or a window it leaves out is removed.
type ReplaceSendingRulesInput struct {
	Revision       int64
	FrequencyCaps  []FrequencyCap
	SendingWindows []SendingWindow
}

// The refusals of a replacement of the sending rules, each reported against the cap or window it
// is about (frequencyCaps.<i>, sendingWindows.<i>).
var (
	// ErrSendingRuleChannel refuses a Channel that is none.
	ErrSendingRuleChannel = validation.RefuseField(fieldname.FrequencyCaps,
		"workspaces: unknown channel", i18n.ErrorsSendingRuleChannelUnknown)
	// ErrSendingRuleCountry refuses a country on a Channel other than a phone's: the country is the
	// number's (TakesCountry).
	ErrSendingRuleCountry = validation.RefuseField(fieldname.FrequencyCaps,
		"workspaces: a country takes a rule only on a phone channel", i18n.ErrorsSendingRuleCountryOffPhone)
	// ErrSendingRuleRepeated refuses a second cap, or a second window, for one Channel and country.
	ErrSendingRuleRepeated = validation.RefuseField(fieldname.FrequencyCaps,
		"workspaces: two rules for one channel and country", i18n.ErrorsSendingRuleRepeated)
	// ErrFrequencyCapPeriod refuses a cap with a maximum and no period, or a period and no maximum.
	ErrFrequencyCapPeriod = validation.RefuseField(fieldname.FrequencyCaps,
		"workspaces: a cap has both a maximum and a period or neither", i18n.ErrorsFrequencyCapPeriod)
	// ErrSendingWindowChannel refuses a window on a Channel that has none (HasSendingWindow).
	ErrSendingWindowChannel = validation.RefuseField(fieldname.SendingWindows,
		"workspaces: the channel has no sending window", i18n.ErrorsSendingWindowChannel)
	// ErrSendingWindowNeverOpens refuses a window open on no day: its messages would wait for ever.
	ErrSendingWindowNeverOpens = validation.RefuseField(fieldname.SendingWindows,
		"workspaces: the sending window never opens", i18n.ErrorsSendingWindowNeverOpens)
	// ErrSendingWindowHours refuses a day whose window does not close after it opens.
	ErrSendingWindowHours = validation.RefuseField(fieldname.SendingWindows,
		"workspaces: a sending window closes after it opens", i18n.ErrorsSendingWindowHours)
	// ErrSendingWindowDayRepeated refuses a day listed twice in one window, or one that is no day.
	ErrSendingWindowDayRepeated = validation.RefuseField(fieldname.SendingWindows,
		"workspaces: a day is listed twice in a sending window", i18n.ErrorsSendingWindowDayRepeated)
)

// TakesCountry reports whether rules of the Channel may be set for a country, by the number's
// country: only on a phone Channel (ADR 0134, ADR 0137).
func TakesCountry(ch channels.Channel) bool {
	switch ch {
	case channels.SMS:
		return true
	case channels.Email, channels.Telegram:
		return false
	}
	return false
}

// HasSendingWindow reports whether marketing messages on the Channel wait for a Sending window
// (ADR 0134): SMS does; email and Telegram, which no law restricts by the hour, do not.
func HasSendingWindow(ch channels.Channel) bool {
	switch ch {
	case channels.SMS:
		return true
	case channels.Email, channels.Telegram:
		return false
	}
	return false
}

// startingCap is the Frequency cap a new Workspace has on the Channel (ADR 0137), nil for none:
// three marketing texts per 24 hours on SMS, stricter than the strictest caps on marketing texts
// since it counts every subject, and none on email and Telegram.
func startingCap(ch channels.Channel) *FrequencyCap {
	switch ch {
	case channels.SMS:
		return &FrequencyCap{Channel: ch, MaxMessages: new(3), PeriodMinutes: new(24 * 60)}
	case channels.Email, channels.Telegram:
		return nil
	}
	return nil
}

// startingWindow is the Sending window a new Workspace has on the Channel (ADR 0134), nil for none:
// 8:00 to 20:00 every day on SMS, inside the narrowest hours the law sets in the places that set
// them; email and Telegram have none.
func startingWindow(ch channels.Channel) *SendingWindow {
	switch ch {
	case channels.SMS:
		hours := make([]OpenHours, 0, len(localcalendar.Weekdays))
		for _, day := range localcalendar.Weekdays {
			hours = append(hours, OpenHours{Day: day, OpensAt: 8 * 60, ClosesAt: 20 * 60})
		}
		return &SendingWindow{Channel: ch, Hours: hours}
	case channels.Email, channels.Telegram:
		return nil
	}
	return nil
}

// everyChannel is every Channel, in the order the columns list them.
func everyChannel() []channels.Channel {
	values := channels.Channel("").Values()
	out := make([]channels.Channel, len(values))
	for i, v := range values {
		out[i] = channels.Channel(v)
	}
	return out
}

// writeStartingRulesIn writes the sending rules a new Workspace starts with (startingCap,
// startingWindow) in tx, the transaction that creates it.
func writeStartingRulesIn(ctx context.Context, tx *ent.Client, workspaceID int64) error {
	for _, ch := range everyChannel() {
		if c := startingCap(ch); c != nil {
			err := tx.FrequencyCap.Create().SetWorkspaceID(workspaceID).SetChannel(c.Channel).
				SetNillableMaxMessages(c.MaxMessages).SetNillablePeriodMinutes(c.PeriodMinutes).Exec(ctx)
			if err != nil {
				return fmt.Errorf("write the starting frequency cap: %w", err)
			}
		}
		if w := startingWindow(ch); w != nil {
			err := withHours(tx.SendingWindow.Create().SetWorkspaceID(workspaceID).SetChannel(w.Channel), w.Hours).Exec(ctx)
			if err != nil {
				return fmt.Errorf("write the starting sending window: %w", err)
			}
		}
	}
	return nil
}

// SendingRules reads the sending rules of the scoped Workspace with its revision.
func (m *Module) SendingRules(ctx context.Context, s *ent.Scoped) (SendingRules, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return SendingRules{}, err
	}
	rules, err := sendingRulesOf(ctx, s)
	if err != nil {
		return SendingRules{}, err
	}
	rules.Revision = ws.Revision
	return rules, nil
}

// ReplaceSendingRules replaces the sending rules of the scoped Workspace as a whole (ADR 0134, ADR
// 0137): a cap or a window in.FrequencyCaps or in.SendingWindows leaves out is removed. Each is
// checked first, and the first refusal is returned with nothing written: a Channel that is none
// (ErrSendingRuleChannel), a country on a Channel other than a phone's (ErrSendingRuleCountry) or
// outside the contract's form (bounds.CountryCode), two caps or two windows for one Channel and
// country (ErrSendingRuleRepeated), a cap with a maximum and no period or the reverse
// (ErrFrequencyCapPeriod) or either outside its bounds, a window on a Channel that has none
// (ErrSendingWindowChannel), one that never opens (ErrSendingWindowNeverOpens), a day listed twice
// (ErrSendingWindowDayRepeated), and hours outside the day or that do not close after they open
// (ErrSendingWindowHours). It is an edit made on in.Revision, as every edit of the Workspace's
// settings is (ADR 0119): a change moves the Workspace's revision and is recorded as one
// `workspace.sending_rules_change` Audit entry of the scope's actor naming each list that changed;
// a replacement that changes nothing writes and records nothing, and is still refused on a stale
// revision (ErrRevisionChanged).
func (m *Module) ReplaceSendingRules(ctx context.Context, s *ent.Scoped, in ReplaceSendingRulesInput) (SendingRules, error) {
	caps, err := checkFrequencyCaps(in.FrequencyCaps)
	if err != nil {
		return SendingRules{}, err
	}
	windows, err := checkSendingWindows(in.SendingWindows)
	if err != nil {
		return SendingRules{}, err
	}
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		before, err := sendingRulesOf(ctx, ts)
		if err != nil {
			return err
		}
		diff := map[string]any{}
		if !slices.EqualFunc(before.FrequencyCaps, caps, sameCap) {
			diff[DiffFrequencyCaps] = events.Change(before.FrequencyCaps, caps)
		}
		if !slices.EqualFunc(before.SendingWindows, windows, sameWindow) {
			diff[DiffSendingWindows] = events.Change(before.SendingWindows, windows)
		}
		if len(diff) == 0 {
			_, err := edit(ts, in.Revision).Save(ctx)
			return err
		}
		// The rows live beside the Workspace, so its revision is moved by touching it: a form read
		// before this change is refused, as for any other of its settings.
		ws, err := edit(ts, in.Revision).SetUpdatedAt(m.now()).Save(ctx)
		if err != nil {
			return err
		}
		if err := replaceRows(ctx, ts, caps, windows); err != nil {
			return err
		}
		return events.RecordAudit(ctx, pub, events.WorkspaceAudit(ws, ts.Actor(), events.ActionWorkspaceSendingRulesChange, diff))
	})
	if err != nil {
		return SendingRules{}, err
	}
	return m.SendingRules(ctx, s)
}

// replaceRows deletes the scoped Workspace's caps and windows and writes caps and windows in their
// place, on ts, the transaction of the replacement.
func replaceRows(ctx context.Context, ts *ent.Scoped, caps []FrequencyCap, windows []SendingWindow) error {
	if _, err := ts.FrequencyCap().Delete().Exec(ctx); err != nil {
		return fmt.Errorf("delete the frequency caps: %w", err)
	}
	if _, err := ts.SendingWindow().Delete().Exec(ctx); err != nil {
		return fmt.Errorf("delete the sending windows: %w", err)
	}
	for _, c := range caps {
		err := ts.FrequencyCap().Create().SetChannel(c.Channel).SetNillableCountry(c.Country).
			SetNillableMaxMessages(c.MaxMessages).SetNillablePeriodMinutes(c.PeriodMinutes).Exec(ctx)
		if err != nil {
			return fmt.Errorf("write a frequency cap: %w", err)
		}
	}
	for _, w := range windows {
		err := withHours(ts.SendingWindow().Create().SetChannel(w.Channel).SetNillableCountry(w.Country), w.Hours).Exec(ctx)
		if err != nil {
			return fmt.Errorf("write a sending window: %w", err)
		}
	}
	return nil
}

// sendingRulesOf reads the caps and the windows of the scoped Workspace, in their order.
func sendingRulesOf(ctx context.Context, s *ent.Scoped) (SendingRules, error) {
	capRows, err := s.FrequencyCap().Query().
		Order(frequencycap.ByChannel(), frequencycap.ByCountry(entsql.OrderNullsFirst())).All(ctx)
	if err != nil {
		return SendingRules{}, err
	}
	windowRows, err := s.SendingWindow().Query().
		Order(sendingwindow.ByChannel(), sendingwindow.ByCountry(entsql.OrderNullsFirst())).All(ctx)
	if err != nil {
		return SendingRules{}, err
	}
	rules := SendingRules{FrequencyCaps: make([]FrequencyCap, 0, len(capRows)), SendingWindows: make([]SendingWindow, 0, len(windowRows))}
	for _, r := range capRows {
		rules.FrequencyCaps = append(rules.FrequencyCaps,
			FrequencyCap{Channel: r.Channel, Country: r.Country, MaxMessages: r.MaxMessages, PeriodMinutes: r.PeriodMinutes})
	}
	for _, r := range windowRows {
		rules.SendingWindows = append(rules.SendingWindows, SendingWindow{Channel: r.Channel, Country: r.Country, Hours: WindowHours(r)})
	}
	return rules, nil
}

// checkFrequencyCaps checks every cap and answers them in the order the rows are read in.
func checkFrequencyCaps(caps []FrequencyCap) ([]FrequencyCap, error) {
	if err := validation.Items(fieldname.FrequencyCaps, len(caps), 0, bounds.FrequencyCapsMaxItems); err != nil {
		return nil, err
	}
	out := make([]FrequencyCap, 0, len(caps))
	seen := map[ruleKey]bool{}
	for i, c := range caps {
		at := validation.Nested(fieldname.FrequencyCaps, strconv.Itoa(i))
		key, err := checkRuleKey(at, c.Channel, c.Country)
		if err != nil {
			return nil, err
		}
		if seen[key] {
			return nil, ErrSendingRuleRepeated.On(at)
		}
		seen[key] = true
		if (c.MaxMessages == nil) != (c.PeriodMinutes == nil) {
			return nil, ErrFrequencyCapPeriod.On(at)
		}
		if c.MaxMessages != nil {
			if err := validation.Range(validation.Nested(at, fieldname.MaxMessages), *c.MaxMessages,
				bounds.FrequencyCapMaxMessagesMin, bounds.FrequencyCapMaxMessagesMax); err != nil {
				return nil, err
			}
			if err := validation.Range(validation.Nested(at, fieldname.PeriodMinutes), *c.PeriodMinutes,
				bounds.FrequencyCapPeriodMinutesMin, bounds.FrequencyCapPeriodMinutesMax); err != nil {
				return nil, err
			}
		}
		out = append(out, FrequencyCap{Channel: c.Channel, Country: c.Country, MaxMessages: c.MaxMessages, PeriodMinutes: c.PeriodMinutes})
	}
	slices.SortFunc(out, func(a, b FrequencyCap) int { return compareRuleKeys(a.Channel, a.Country, b.Channel, b.Country) })
	return out, nil
}

// checkSendingWindows checks every window and answers them in the order the rows are read in,
// each one's hours Monday first.
func checkSendingWindows(windows []SendingWindow) ([]SendingWindow, error) {
	if err := validation.Items(fieldname.SendingWindows, len(windows), 0, bounds.SendingWindowsMaxItems); err != nil {
		return nil, err
	}
	out := make([]SendingWindow, 0, len(windows))
	seen := map[ruleKey]bool{}
	for i, w := range windows {
		at := validation.Nested(fieldname.SendingWindows, strconv.Itoa(i))
		key, err := checkRuleKey(at, w.Channel, w.Country)
		if err != nil {
			return nil, err
		}
		if !HasSendingWindow(w.Channel) {
			return nil, ErrSendingWindowChannel.On(at)
		}
		if seen[key] {
			return nil, ErrSendingRuleRepeated.On(at)
		}
		seen[key] = true
		hours, err := checkHours(at, w.Hours)
		if err != nil {
			return nil, err
		}
		out = append(out, SendingWindow{Channel: w.Channel, Country: w.Country, Hours: hours})
	}
	slices.SortFunc(out, func(a, b SendingWindow) int { return compareRuleKeys(a.Channel, a.Country, b.Channel, b.Country) })
	return out, nil
}

// checkHours checks the hours of the window at, and answers them Monday first.
func checkHours(at string, hours []OpenHours) ([]OpenHours, error) {
	field := validation.Nested(at, fieldname.Hours)
	if err := validation.Items(field, len(hours), 0, bounds.SendingWindowHoursMaxItems); err != nil {
		return nil, err
	}
	if len(hours) == 0 {
		return nil, ErrSendingWindowNeverOpens.On(at)
	}
	seen := map[localcalendar.Weekday]bool{}
	for j, h := range hours {
		day := validation.Nested(field, strconv.Itoa(j))
		if !slices.Contains(localcalendar.Weekdays, h.Day) || seen[h.Day] {
			return nil, ErrSendingWindowDayRepeated.On(day)
		}
		seen[h.Day] = true
		for _, minute := range []struct {
			name  string
			value int
		}{{fieldname.OpensAt, h.OpensAt}, {fieldname.ClosesAt, h.ClosesAt}} {
			if err := validation.Range(validation.Nested(day, minute.name), minute.value,
				bounds.MinuteOfDayMin, bounds.MinuteOfDayMax); err != nil {
				return nil, err
			}
		}
		if h.ClosesAt <= h.OpensAt {
			return nil, ErrSendingWindowHours.On(day)
		}
	}
	out := slices.Clone(hours)
	slices.SortFunc(out, func(a, b OpenHours) int {
		return cmp.Compare(slices.Index(localcalendar.Weekdays, a.Day), slices.Index(localcalendar.Weekdays, b.Day))
	})
	return out, nil
}

// ruleKey is what a Workspace has one cap and one window per: a Channel and a country, "" for the
// Channel's own.
type ruleKey struct {
	channel channels.Channel
	country string
}

// checkRuleKey checks the Channel and the country of the cap or window at.
func checkRuleKey(at string, ch channels.Channel, country *string) (ruleKey, error) {
	if !ch.Valid() {
		return ruleKey{}, ErrSendingRuleChannel.On(at)
	}
	if country == nil {
		return ruleKey{channel: ch}, nil
	}
	if err := validation.Pattern(validation.Nested(at, fieldname.Country), *country, bounds.CountryCodePattern); err != nil {
		return ruleKey{}, err
	}
	if !TakesCountry(ch) {
		return ruleKey{}, ErrSendingRuleCountry.On(at)
	}
	return ruleKey{channel: ch, country: *country}, nil
}

// compareRuleKeys orders rules as the rows are read: by Channel, then the Channel's own row before
// a country's, the countries in order.
func compareRuleKeys(aCh channels.Channel, aCountry *string, bCh channels.Channel, bCountry *string) int {
	if c := cmp.Compare(aCh, bCh); c != 0 {
		return c
	}
	switch {
	case aCountry == nil && bCountry == nil:
		return 0
	case aCountry == nil:
		return -1
	case bCountry == nil:
		return 1
	}
	return cmp.Compare(*aCountry, *bCountry)
}

func sameCap(a, b FrequencyCap) bool {
	return a.Channel == b.Channel && samePtr(a.Country, b.Country) &&
		samePtr(a.MaxMessages, b.MaxMessages) && samePtr(a.PeriodMinutes, b.PeriodMinutes)
}

func sameWindow(a, b SendingWindow) bool {
	return a.Channel == b.Channel && samePtr(a.Country, b.Country) && slices.Equal(a.Hours, b.Hours)
}

func samePtr[T comparable](a, b *T) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// WindowHours is the hours of the days a Sending window row is open on, Monday first: the one
// reading of its columns.
func WindowHours(r *ent.SendingWindow) []OpenHours {
	days := []struct {
		day           localcalendar.Weekday
		opens, closes *int
	}{
		{localcalendar.Monday, r.MondayOpens, r.MondayCloses},
		{localcalendar.Tuesday, r.TuesdayOpens, r.TuesdayCloses},
		{localcalendar.Wednesday, r.WednesdayOpens, r.WednesdayCloses},
		{localcalendar.Thursday, r.ThursdayOpens, r.ThursdayCloses},
		{localcalendar.Friday, r.FridayOpens, r.FridayCloses},
		{localcalendar.Saturday, r.SaturdayOpens, r.SaturdayCloses},
		{localcalendar.Sunday, r.SundayOpens, r.SundayCloses},
	}
	hours := make([]OpenHours, 0, len(days))
	for _, d := range days {
		if d.opens != nil && d.closes != nil {
			hours = append(hours, OpenHours{Day: d.day, OpensAt: *d.opens, ClosesAt: *d.closes})
		}
	}
	return hours
}

// windowCreate is a create of a Sending window row, through the raw client or the scope: what sets
// the hours of each day.
type windowCreate[T any] interface {
	SetMondayOpens(minute int) T
	SetMondayCloses(minute int) T
	SetTuesdayOpens(minute int) T
	SetTuesdayCloses(minute int) T
	SetWednesdayOpens(minute int) T
	SetWednesdayCloses(minute int) T
	SetThursdayOpens(minute int) T
	SetThursdayCloses(minute int) T
	SetFridayOpens(minute int) T
	SetFridayCloses(minute int) T
	SetSaturdayOpens(minute int) T
	SetSaturdayCloses(minute int) T
	SetSundayOpens(minute int) T
	SetSundayCloses(minute int) T
}

// withHours is the create c with the hours of each day the window is open on; a day hours do not
// list stays closed (its columns null).
func withHours[T windowCreate[T]](c T, hours []OpenHours) T {
	for _, h := range hours {
		switch h.Day {
		case localcalendar.Monday:
			c = c.SetMondayOpens(h.OpensAt).SetMondayCloses(h.ClosesAt)
		case localcalendar.Tuesday:
			c = c.SetTuesdayOpens(h.OpensAt).SetTuesdayCloses(h.ClosesAt)
		case localcalendar.Wednesday:
			c = c.SetWednesdayOpens(h.OpensAt).SetWednesdayCloses(h.ClosesAt)
		case localcalendar.Thursday:
			c = c.SetThursdayOpens(h.OpensAt).SetThursdayCloses(h.ClosesAt)
		case localcalendar.Friday:
			c = c.SetFridayOpens(h.OpensAt).SetFridayCloses(h.ClosesAt)
		case localcalendar.Saturday:
			c = c.SetSaturdayOpens(h.OpensAt).SetSaturdayCloses(h.ClosesAt)
		case localcalendar.Sunday:
			c = c.SetSundayOpens(h.OpensAt).SetSundayCloses(h.ClosesAt)
		}
	}
	return c
}
