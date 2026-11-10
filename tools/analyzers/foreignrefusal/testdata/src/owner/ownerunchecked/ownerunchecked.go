package ownerunchecked

import "owner"

// Write is an unchecked operation of the owner: it refuses in the owner's words.
func Write(name string) error {
	if name == "" {
		return owner.ErrBlank
	}
	return nil
}
