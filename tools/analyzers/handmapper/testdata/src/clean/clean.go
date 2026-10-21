package clean

import (
	"example/ent"
	"example/gen/site"
)

// Converter maps ent rows to resources.
//
// goverter:converter
// goverter:extend emailOf
type Converter interface {
	Contact(c *ent.Contact) site.ContactResource
	// goverter:map . Mail | mailOf
	Mail(c *ent.Contact) site.ContactResource
}

// mailOf is named by a map line of the converter.
func mailOf(c *ent.Contact) site.Opt[string] { return site.Opt[string]{Value: c.Email} }

// emailOf is a registered converter helper.
func emailOf(c *ent.Contact) site.Opt[string] { return site.Opt[string]{Value: c.Email} }

// A handler takes the scope, which is not an entity.
func list(s *ent.Scoped) site.ContactResource { return site.ContactResource{} }

// No resource comes out.
func name(c *ent.Contact) string { return c.Email }

// No entity goes in.
func build(email string) site.ContactResource { return site.ContactResource{Email: email} }

var mapper Converter

// patched maps through the converter and attaches what it cannot see.
func patched(c *ent.Contact) site.ContactResource {
	res := mapper.Contact(c)
	res.Email += "!"
	return res
}

// loaded wraps a function that maps through the converter.
func loaded(c *ent.Contact) (site.ContactResource, error) { return patched(c), nil }

type handlers struct{}

// one delegates to a method that maps through the converter.
func (h *handlers) one(c *ent.Contact, _ *ent.Scoped) (site.ContactResource, error) {
	return h.resource(c, 1)
}

func (h *handlers) resource(c *ent.Contact, _ int) (site.ContactResource, error) {
	return mapper.Contact(c), nil
}
