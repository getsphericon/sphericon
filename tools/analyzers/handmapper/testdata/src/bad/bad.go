package bad

import (
	"example/ent"
	"example/gen/site"
)

func contactResource(c *ent.Contact) site.ContactResource { // want `rule 1: contactResource maps.*fix: .*goverter`
	return site.ContactResource{Email: c.Email}
}

func contactResources(cs []*ent.Contact) ([]site.ContactResource, error) { // want `rule 1: contactResources maps`
	return nil, nil
}

type H struct{}

func (H) optional(c *ent.Contact) site.Opt[site.ContactResource] { // want `rule 1: optional maps`
	return site.Opt[site.ContactResource]{}
}
