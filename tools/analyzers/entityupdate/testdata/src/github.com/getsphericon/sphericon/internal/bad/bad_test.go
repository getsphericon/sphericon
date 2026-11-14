package bad

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/ent"
)

// Test code is read too.
func TestUpdate(t *testing.T) {
	_ = (&ent.Tag{}).Update().Exec(context.Background()) // want `^rule 18: Update on a loaded Tag`
}
