package bad

import (
	"testing"

	"github.com/getsphericon/sphericon/ent"
)

func TestRaw(t *testing.T) {
	c := &ent.Client{}
	c.Tag.Create().SetName("a").SaveX() // want `^rule 10: a test creates a row inline.*; fix: add a fixture row \(see internal/AGENTS.md\)`
	c.Tag.CreateBulk(nil)               // want `^rule 10:`
}

func TestScoped(t *testing.T) {
	s := &ent.Scoped{}
	s.Tag().Create().SaveX() // want `^rule 10:`

	// test writes:
	s.Tag().Create().SaveX() // want `^rule 10:`

	// An ordinary comment is not a reason.
	s.Tag().Create().SaveX() // want `^rule 10:`
}
