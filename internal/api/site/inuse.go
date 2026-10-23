package site

import (
	"net/http"

	"github.com/samber/lo"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/narrow"
)

// inUseProblem renders a removal refused because live work uses the row (ADR 0049): a 409
// whose `usedBy` names the users. Every refused removal of this surface goes through it.
func inUseProblem(e *inuse.Error) siteapi.InUseProblemDetails {
	base := problem(http.StatusConflict, e.Error())
	return siteapi.InUseProblemDetails{
		Status: base.Status,
		Title:  base.Title,
		Detail: base.Detail,
		UsedBy: lo.Map(e.Users, func(u inuse.User, _ int) siteapi.UsedBy {
			by := siteapi.UsedBy{Kind: siteapi.UsedByKind(u.Kind), ID: siteapi.EntityId(u.ID), Name: u.Name}
			if u.Count > 0 {
				by.Count = siteapi.NewOptInt32(narrow.Int32(u.Count))
			}
			return by
		}),
	}
}
