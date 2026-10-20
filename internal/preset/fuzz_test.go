package preset_test

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"

	"github.com/getsphericon/sphericon/internal/preset"
)

// A Preset file is operator input: whatever it holds, Parse answers with a Preset or an error,
// never both and never a panic.
func FuzzParseAnswersWithAPresetOrAnError(f *testing.F) {
	seeds, err := filepath.Glob(filepath.Join("builtin", "*.yaml"))
	if err != nil {
		f.Fatal(err)
	}
	for _, path := range seeds {
		data, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Add([]byte(""))
	f.Add([]byte("name: x"))

	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := preset.Parse("fuzz.yaml", data)

		require.NotEqualf(t, p == nil, err == nil, "Parse(%q) returned preset %v and error %v", data, p, err)
	})
}
