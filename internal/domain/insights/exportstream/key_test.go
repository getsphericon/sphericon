package exportstream_test

import "github.com/getsphericon/sphericon/internal/kernel/publicid"

// The blank variable sets an id key while the package is initialized: a cursor seals its row's id (publicid), and these tests
// build no database.
var _ = func() bool {
	if err := publicid.Use(make([]byte, 16)); err != nil {
		panic(err)
	}
	return true
}()
