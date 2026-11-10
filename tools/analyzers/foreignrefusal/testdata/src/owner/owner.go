package owner

import (
	"errors"

	"internal/kernel/validation"
)

// ErrBlank is the owner's refusal: the owner returns it, which is its own word.
var ErrBlank = validation.Refuse("owner: blank")

// errPlain is no refusal: another package may name such an error freely.
var errPlain = errors.New("owner: plain")

// Plain is errPlain.
var Plain = errPlain

func Check(name string) error {
	if name == "" {
		return ErrBlank
	}
	return nil
}
