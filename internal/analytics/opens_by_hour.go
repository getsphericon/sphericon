package analytics

import (
	"context"

	"entgo.io/ent/dialect/sql"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
)

// HourCell is the human opens of one weekday and hour of day in the Workspace time zone.
// Weekday is ISO: 1 is Monday, 7 is Sunday.
type HourCell struct {
	Weekday, Hour, Opens int
}

// OpensByHour is the Opens by hour report: a cell for every weekday and hour, empty ones zero,
// ordered Monday 00:00 first.
type OpensByHour struct {
	Cells []HourCell
	Total int
}

// OpensByHour counts the human opens of the window by hour of day and day of week, both read in
// the Workspace time zone (ADR 0057). Machine opens (ADR 0053) are in no cell. An open happened
// when `occurred_at` says, falling back to the creation time (ADR 0044).
func (m *Module) OpensByHour(ctx context.Context, s *ent.Scoped, w Window) (*OpensByHour, error) {
	days, err := m.days(ctx, s, w)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Weekday int `sql:"weekday"`
		Hour    int `sql:"hour"`
		Opens   int `sql:"opens"`
	}
	err = s.Event().Query().
		Where(event.ActionEQ(events.NameEmailOpened)).
		Modify(func(sel *sql.Selector) {
			happened := "COALESCE(" + sel.C(event.FieldOccurredAt) + ", " + sel.C(event.FieldCreatedAt) + ")"
			events.HumanOnly(sel)
			sel.Where(sql.And(sql.GTE(happened, days.Since), sql.LT(happened, days.Until)))
			local := func(field string) sql.Querier {
				// raw sql: ent has no AT TIME ZONE or EXTRACT expression; the zone is a bound argument
				return sql.ExprFunc(func(b *sql.Builder) {
					b.WriteString("EXTRACT(" + field + " FROM " + happened + " AT TIME ZONE ").Arg(days.Loc.String()).WriteString(")::int")
				})
			}
			sel.Select(sql.As("COUNT(*)", "opens")).
				AppendSelectExprAs(local("ISODOW"), "weekday").
				AppendSelectExprAs(local("HOUR"), "hour").
				GroupBy("weekday", "hour")
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	var grid [7][24]int
	report := &OpensByHour{}
	for _, r := range rows {
		grid[r.Weekday-1][r.Hour] = r.Opens
		report.Total += r.Opens
	}
	for d := range grid {
		for h, n := range grid[d] {
			report.Cells = append(report.Cells, HourCell{Weekday: d + 1, Hour: h, Opens: n})
		}
	}
	return report, nil
}
