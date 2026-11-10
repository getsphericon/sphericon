package jobs_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

var errRefused = validation.Refuse("test: refused", i18n.ErrorsNameEmpty)

// A refusal of a module ends the job for good, whatever the job; any other error is left to
// river's retry (ADR 0107).
func TestARefusalEndsTheJobAndAnyOtherErrorRetriesIt(t *testing.T) {
	m := &jobs.RefusalsEnd{}
	work := func(err error) error {
		return m.Work(context.Background(), nil, func(context.Context) error { return err })
	}

	var cancel *river.JobCancelError
	require.ErrorAs(t, work(fmt.Errorf("wrapped: %w", errRefused)), &cancel)
	require.ErrorAs(t, work(validation.Fields(errRefused.On("name"))), &cancel)

	broken := errors.New("db down")
	assert.Equal(t, broken, work(broken), "a fault is left to river's retry")
	assert.NoError(t, work(nil))
}
