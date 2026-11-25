package condition_test

import "github.com/getsphericon/sphericon/internal/kernel/publicid"

// The blank variable sets an id key while the package is initialized: a sent-by test names its
// Broadcast or Automation by its public id (publicid), and the tests that build no database read
// one too. A test that builds the Instance sets the fixtures' key (testhelper.Setup).
var _ = func() bool {
	if err := publicid.Use(make([]byte, 16)); err != nil {
		panic(err)
	}
	return true
}()
