// Package linkclicks counts the clicks of the emails of one Broadcast or Automation, or of the
// whole Workspace, by link. It is a leaf the per-owner reports and the Clicks by link report share:
// the count reads the `email.clicked` Events, machine clicks left out (ADR 0053), and takes a
// scoped client like every Workspace read.
package linkclicks

import (
	"context"
	"strconv"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
)

// Clicks is how often one link was clicked.
type Clicks struct {
	URL    string `json:"url"`
	Clicks int    `json:"clicks"`
}

// OwnerKind is what carried a clicked link.
type OwnerKind string

const (
	OwnerBroadcast  OwnerKind = "broadcast"
	OwnerAutomation OwnerKind = "automation"
)

// OwnerClicks is how often one link was clicked in the emails of one Broadcast or Automation.
type OwnerClicks struct {
	URL    string
	Kind   OwnerKind
	ID     int64
	Clicks int
}

// Between is the half-open span [Since, Until) the clicks happened in; the zero value is all
// the retained history. An Event happened at its `occurred_at`, falling back to its creation
// time (ADR 0044).
type Between struct {
	Since, Until time.Time
}

// ByLink counts the `email.clicked` Events that owner narrows to (the Events that carry one
// Broadcast or Automation) within the span, by link, most clicked first. Every click is an
// Event, so these are repeat counts that cover the Event retention window.
func ByLink(ctx context.Context, s *ent.Scoped, in Between, owner func(*sql.Selector)) ([]Clicks, error) {
	rows := []Clicks{}
	err := clicks(s, in, func(sel *sql.Selector) {
		owner(sel)
		url := urlOf(sel)
		sel.Select(sql.As(url, "url"), sql.As(sql.Count("*"), "clicks")).GroupBy(url)
		// raw sql: order by the aggregate's alias, which ent's order terms cannot name here.
		sel.OrderExpr(sql.Expr("clicks DESC, " + url))
	}).
		Scan(ctx, &rows)
	return rows, err
}

// ByOwner counts the human `email.clicked` Events of the whole Workspace within the span, by
// link and by the Broadcast or Automation that carried it, most clicked first. A click that
// carries neither (a Transactional send) belongs to no report and is left out.
func ByOwner(ctx context.Context, s *ent.Scoped, in Between) ([]OwnerClicks, error) {
	var rows []struct {
		URL          string  `json:"url"`
		BroadcastID  *string `json:"broadcast_id"`
		AutomationID *string `json:"automation_id"`
		Clicks       int     `json:"clicks"`
	}
	err := clicks(s, in, func(sel *sql.Selector) {
		props := sel.C(event.FieldProperties)
		// raw sql: the owner ids are keys of the JSON properties, read as text and grouped on.
		broadcast, automation := "("+props+"->>'broadcastId')", "("+props+"->>'automationId')"
		// raw sql: a click of a Broadcast or an Automation carries one of the two ids.
		sel.Where(sql.ExprP("(" + broadcast + " IS NOT NULL OR " + automation + " IS NOT NULL)"))
		url := urlOf(sel)
		sel.Select(
			sql.As(url, "url"), sql.As(broadcast, "broadcast_id"), sql.As(automation, "automation_id"),
			sql.As(sql.Count("*"), "clicks"),
		).GroupBy(url, broadcast, automation)
		// raw sql: order by the aggregate's alias, which ent's order terms cannot name here.
		sel.OrderExpr(sql.Expr("clicks DESC, " + url))
	}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	owners := make([]OwnerClicks, 0, len(rows))
	for _, r := range rows {
		kind, raw := OwnerBroadcast, r.BroadcastID
		if raw == nil {
			kind, raw = OwnerAutomation, r.AutomationID
		}
		id, err := strconv.ParseInt(*raw, 10, 64)
		if err != nil {
			continue // an owner id that is no number is no Broadcast or Automation
		}
		owners = append(owners, OwnerClicks{URL: r.URL, Kind: kind, ID: id, Clicks: r.Clicks})
	}
	return owners, nil
}

// Total is the clicks over all the links.
func Total(links []Clicks) int {
	total := 0
	for _, l := range links {
		total += l.Clicks
	}
	return total
}

// clicks is the human click Events inside the span.
func clicks(s *ent.Scoped, in Between, shape func(*sql.Selector)) *ent.EventSelect {
	return s.Event().Query().
		Where(event.ActionEQ(events.NameEmailClicked)).
		Modify(func(sel *sql.Selector) {
			events.HumanOnly(sel)
			if in != (Between{}) {
				events.MomentBetween(sel, in.Since, in.Until)
			}
			shape(sel)
		})
}

// urlOf is the link of a click: a key of the JSON properties, read as text.
func urlOf(sel *sql.Selector) string {
	// raw sql: the link is a key of the JSON properties, read as text and grouped on.
	return "(" + sel.C(event.FieldProperties) + "->>'url')"
}
