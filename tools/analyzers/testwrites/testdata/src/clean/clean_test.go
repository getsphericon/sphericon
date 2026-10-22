package clean

import (
	"testing"

	"github.com/getsphericon/sphericon/ent"
)

func TestWriteIsTheSubject(t *testing.T) {
	s := &ent.Scoped{}
	// test writes: the scoped wrapper's Create is the subject
	s.Tag().Create().SetName("a").SaveX()

	s.Tag().Create().SaveX() // test writes: trailing reason

	// test writes: a chain split over lines is one statement
	s.Tag().
		Create().
		SetName("a").
		SaveX()
}
