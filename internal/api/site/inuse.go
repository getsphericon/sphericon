package site

import (
	"net/http"

	"github.com/samber/lo"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/inuse"
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
			return siteapi.UsedBy{Kind: siteapi.UsedByKind(u.Kind), ID: siteapi.EntityId(u.ID), Name: u.Name}
		}),
	}
}
