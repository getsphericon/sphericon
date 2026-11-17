package clean

import (
	"errors"

	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/domain/owner"
)

// errOwn is this package's own refusal: returning it is its own word.
var errOwn = validation.Refuse("clean: own")

func Read(name string) (string, error) {
	err := owner.Check(name)
	switch {
	case errors.Is(err, owner.ErrBlank):
		return "blank", nil
	case errors.Is(err, owner.Plain):
		return "plain", nil
	case err != nil:
		return "", err
	}
	if name == "x" {
		return "", errOwn
	}
	return name, nil
}
