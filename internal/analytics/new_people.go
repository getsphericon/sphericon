package analytics

import (
	"context"

	"entgo.io/ent/dialect/sql"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/reportday"
)

// NewPeople is the New people report: the Contacts created in the window, per Report day and
// by Contact source.
type NewPeople struct {
	// Total is every Contact created in the window.
	Total  int
	Series []NewPeoplePoint
}

// NewPeoplePoint is one Report day of the New people report, zero-filled across the window.
// Unknown counts the Contacts that predate the Contact source.
type NewPeoplePoint struct {
	Date                          string
	Manual, API, Tracker, Unknown int
}

// NewPeople reports the Contacts of the Workspace behind s created in the window.
func (m *Module) NewPeople(ctx context.Context, s *ent.Scoped, w Window) (*NewPeople, error) {
	days, err := m.days(ctx, s, w)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Day    string          `sql:"day"`
		Source *contact.Source `sql:"source"`
		Count  int             `sql:"count"`
	}
	err = s.Contact().Query().
		Where(contact.CreatedAtGTE(days.Since), contact.CreatedAtLT(days.Until)).
		Modify(func(sel *sql.Selector) {
			sel.Select(sel.C(contact.FieldSource), sql.As("COUNT(*)", "count")).
				AppendSelectExprAs(reportday.Label(sel.C(contact.FieldCreatedAt), days.Loc), "day").
				GroupBy("day", sel.C(contact.FieldSource))
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	byDay := make(map[string]NewPeoplePoint, len(days.Dates))
	report := &NewPeople{}
	for _, r := range rows {
		p := byDay[r.Day]
		switch {
		case r.Source == nil:
			p.Unknown += r.Count
		case *r.Source == contact.SourceManual:
			p.Manual += r.Count
		case *r.Source == contact.SourceAPI:
			p.API += r.Count
		case *r.Source == contact.SourceTracker:
			p.Tracker += r.Count
		}
		byDay[r.Day] = p
		report.Total += r.Count
	}
	report.Series = make([]NewPeoplePoint, 0, len(days.Dates))
	for _, day := range days.Dates {
		p := byDay[day]
		p.Date = day
		report.Series = append(report.Series, p)
	}
	return report, nil
}
