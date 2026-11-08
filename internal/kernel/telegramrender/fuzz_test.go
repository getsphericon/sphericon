package telegramrender_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/kernel/telegramrender"
)

func FuzzSanitize(f *testing.F) {
	for _, markup := range []string{
		`<a href="https://e.com"><b><i><b><b><p><a href="https://e.com"></i>`,
		`<a href="https://e.com"><b>x</b></a>`,
		`<pre>\n&lt;b&gt;</pre>`,
		`<script>x</script>&lt;&amp;`,
	} {
		f.Add(markup)
	}
	f.Fuzz(func(t *testing.T, markup string) {
		out := telegramrender.Sanitize(markup)
		assert.Equal(t, out, telegramrender.Sanitize(out), "sanitizing its own output is stable")
	})
}
