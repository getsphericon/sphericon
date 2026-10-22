package clean

import "github.com/getsphericon/sphericon/ent"

// Outside a test file a Create is not this check's business.
func Seed(c *ent.Client) { c.Tag.Create().SaveX() }
