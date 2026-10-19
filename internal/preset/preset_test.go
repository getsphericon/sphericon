package preset_test

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/preset"
)

// The only place a transform or a verification is tested: every Preset carries its own
// cases, and one test runs them all.
func TestPresetsPassTheirOwnTests(t *testing.T) {
	custom, err := preset.Load(os.DirFS("testdata/custom"))
	require.NoError(t, err)

	names := custom.Names()
	require.Contains(t, names, "generic")
	require.Contains(t, names, "hostile")
	require.Contains(t, names, "ses")
	for _, name := range names {
		p, _ := custom.Get(name)
		t.Run(name, func(t *testing.T) {
			require.NoError(t, p.Test(t.Context()))
		})
	}
}

func TestLoadStopsOnABrokenFile(t *testing.T) {
	valid := `
version: 1
name: ok
params: {type: object}
verify: {kind: standard_webhooks, secret: secret}
transform: "[]"
tests:
  - {name: unsigned, request: {body: "{}"}, expect: {refused: unauthentic}}
`
	cases := map[string]struct {
		files map[string]string
		want  []string
	}{
		"not yaml": {
			files: map[string]string{"bad.yaml": "version: 1\nname: [\n"},
			want:  []string{"bad.yaml:"},
		},
		"schema violation names the line": {
			files: map[string]string{"bad.yaml": "version: 1\nname: ok\nparams: {type: object}\nverify:\n  kind: token\n  secret: s\n  header: Not A Header\ntransform: '[]'\ntests:\n  - {name: t, request: {body: '{}'}, expect: {refused: unauthentic}}\n"},
			want:  []string{"bad.yaml:7"},
		},
		"unknown version": {
			files: map[string]string{"bad.yaml": "version: 2\nname: ok\n"},
			want:  []string{"bad.yaml:"},
		},
		"transform that does not compile": {
			files: map[string]string{"bad.yaml": "version: 1\nname: ok\nparams: {type: object}\nverify: {kind: standard_webhooks, secret: s}\ntransform: '[.body |'\ntests:\n  - {name: t, request: {body: '{}'}, expect: {refused: unauthentic}}\n"},
			want:  []string{"bad.yaml:5", "transform"},
		},
		"a program that reads input": {
			files: map[string]string{"bad.yaml": strings.Replace(valid, `transform: "[]"`, `transform: "[input]"`, 1)},
			want:  []string{"bad.yaml:", "input"},
		},
		"a program that calls debug": {
			files: map[string]string{"bad.yaml": strings.Replace(valid, `transform: "[]"`, `transform: "[.body | debug]"`, 1)},
			want:  []string{"bad.yaml:", "debug"},
		},
		"a program that calls stderr": {
			files: map[string]string{"bad.yaml": strings.Replace(valid, `transform: "[]"`, `transform: "[.body | stderr]"`, 1)},
			want:  []string{"bad.yaml:", "stderr"},
		},
		"a program that reads the clock": {
			files: map[string]string{"bad.yaml": strings.Replace(valid, `transform: "[]"`, `transform: "[now | floor | {id: tostring}]"`, 1)},
			want:  []string{"bad.yaml:", "now"},
		},
		"params that are not a schema": {
			files: map[string]string{"bad.yaml": "version: 1\nname: ok\nparams: {type: 7}\nverify: {kind: standard_webhooks, secret: s}\ntransform: '[]'\ntests:\n  - {name: t, request: {body: '{}'}, expect: {refused: unauthentic}}\n"},
			want:  []string{"bad.yaml:3"},
		},
		"name of a built-in": {
			files: map[string]string{"mine.yaml": strings.Replace(valid, "name: ok", "name: generic", 1)},
			want:  []string{"mine.yaml"},
		},
		"name taken by another custom file": {
			files: map[string]string{"a.yaml": valid, "b.yaml": valid},
			want:  []string{"b.yaml", `"ok"`},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fsys := fstest.MapFS{}
			for f, body := range c.files {
				fsys[f] = &fstest.MapFile{Data: []byte(body)}
			}
			_, err := preset.Load(fsys)
			require.Error(t, err)
			for _, w := range c.want {
				require.ErrorContains(t, err, w)
			}
		})
	}
}
