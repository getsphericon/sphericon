// Package linkclicks counts the clicks of the emails of one Broadcast or Automation by link. It is a
// leaf the two reports share: the count reads the `email.clicked` Events, machine clicks left out
// (ADR 0053), and takes a scoped client like every Workspace read.
package linkclicks

import (
	"context"

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

// ByLink counts the `email.clicked` Events that owner narrows to (the Events that carry one
// Broadcast or Automation), by link, most clicked first. Every click is an Event, so these are
// repeat counts that cover the Event retention window.
func ByLink(ctx context.Context, s *ent.Scoped, owner func(*sql.Selector)) ([]Clicks, error) {
	rows := []Clicks{}
	err := s.Event().Query().
		Where(event.ActionEQ(events.NameEmailClicked)).
		Modify(func(sel *sql.Selector) {
			events.HumanOnly(sel)
			owner(sel)
			// raw sql: the link is a key of the JSON properties, read as text and grouped on.
			url := "(" + sel.C(event.FieldProperties) + "->>'url')"
			sel.Select(sql.As(url, "url"), sql.As(sql.Count("*"), "clicks")).GroupBy(url)
			// raw sql: order by the aggregate's alias, which ent's order terms cannot name here.
			sel.OrderExpr(sql.Expr("clicks DESC, " + url))
		}).
		Scan(ctx, &rows)
	return rows, err
}

// Total is the clicks over all the links.
func Total(links []Clicks) int {
	total := 0
	for _, l := range links {
		total += l.Clicks
	}
	return total
}
