package preset

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Open finds the Preset an operator names for `sphericon preset test`: the Preset file at arg,
// or the built-in named arg when there is no such file. A file is parsed on its own, so its
// name may collide with a built-in's and a Self-hoster can test a copy of one.
func Open(arg string) (*Preset, error) {
	data, err := os.ReadFile(arg)
	switch {
	case err == nil:
		return Parse(filepath.Base(arg), data)
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	reg, err := Load(nil)
	if err != nil {
		return nil, err
	}
	p, ok := reg.Get(arg)
	if !ok {
		return nil, fmt.Errorf("%q is neither a file nor a built-in Preset (%s)", arg, strings.Join(reg.Names(), ", "))
	}
	return p, nil
}
