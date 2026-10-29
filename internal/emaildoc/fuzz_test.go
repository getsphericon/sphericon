package emaildoc_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/emaildoc"
)

// acceptingEngine is an editor that takes every document, so a refusal comes from our own checks.
type acceptingEngine struct{}

func (acceptingEngine) Check([]byte) error { return nil }

func (acceptingEngine) Export([]byte) (emaildoc.Rendered, error) { return emaildoc.Rendered{}, nil }

// The document comes from an editor or an agent: whatever bytes arrive, the checks that run
// before a runtime exists neither panic nor answer with anything but a refusal or success.
func FuzzPrepareNeverPanicsAndRefusesCleanly(f *testing.F) {
	seeds, err := filepath.Glob(filepath.Join("testdata", "docs", "*.json"))
	if err != nil {
		f.Fatal(err)
	}
	for _, path := range seeds {
		doc, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(doc)
	}
	f.Add([]byte(`{"type":"doc","content":[]}`))
	f.Add([]byte(`[`))
	r := emaildoc.New(emaildoc.WithEngine(acceptingEngine{}))

	f.Fuzz(func(t *testing.T, doc []byte) {
		err := r.Validate(t.Context(), doc)

		refusedCleanly := err == nil || errors.Is(err, emaildoc.ErrInvalid) || errors.Is(err, emaildoc.ErrTooLarge) ||
			errors.Is(err, emaildoc.ErrTooDeep) || errors.Is(err, emaildoc.ErrTooManyNodes)
		require.Truef(t, refusedCleanly, "%q was refused with an error of no known kind: %v", doc, err)
	})
}
