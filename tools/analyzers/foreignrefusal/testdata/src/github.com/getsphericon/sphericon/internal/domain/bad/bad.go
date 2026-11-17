package bad

import (
	"fmt"

	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/domain/owner"
)

// ErrAlias re-exports the owner's refusal under this package's name.
var ErrAlias = owner.ErrBlank // want `rule 5: owner.ErrBlank is owner's refusal, used here outside errors.Is`

func Refuse(name string) error {
	if name == "" {
		return owner.ErrBlank // want `rule 5: owner.ErrBlank is owner's refusal`
	}
	if name == " " {
		return fmt.Errorf("wrapped: %w", owner.ErrBlank) // want `rule 5: owner.ErrBlank is owner's refusal`
	}
	return validation.ErrShared // want `rule 5: validation.ErrShared is validation's refusal`
}
