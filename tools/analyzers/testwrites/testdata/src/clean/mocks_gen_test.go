package clean

import "github.com/getsphericon/sphericon/ent"

func generated(c *ent.Client) { c.Tag.Create().SaveX() }
