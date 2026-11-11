// Package testhelper stands for the test harness, which builds a fixture's state directly.
package testhelper

import "github.com/getsphericon/sphericon/ent"

func ReopenInvitation(c *ent.Client) {
	c.Invitation().ClearAcceptedAt()
}
