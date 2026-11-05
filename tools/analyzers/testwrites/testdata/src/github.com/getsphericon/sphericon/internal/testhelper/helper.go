package testhelper

import "github.com/getsphericon/sphericon/ent"

// Seed creates a row for every test that calls it, so it gives the reason a test would.
func Seed(c *ent.Client) {
	c.Tag.Create().SaveX() // want `^rule 10:`

	// test writes: the token is minted with the scopes the caller names, which no fixture can know
	c.Tag.Create().SaveX()
}
