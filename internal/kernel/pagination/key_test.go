package pagination_test

import "github.com/getsphericon/sphericon/internal/kernel/publicid"

// The blank variable sets an id key while the package is initialized: an id cursor is a public id
// (publicid).
var _ = func() bool {
	if err := publicid.Use(make([]byte, 16)); err != nil {
		panic(err)
	}
	return true
}()
