package clean

import (
	"errors"

	"internal/kernel/validation"
	"owner"
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
