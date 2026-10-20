package preset

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// Schema is the JSON Schema of the Preset file (draft 2020-12), published for editors
// (`# yaml-language-server: $schema=...`) and the validator the loader uses.
//
//go:embed preset.schema.json
var Schema []byte

//go:embed builtin/*.yaml
var builtinFS embed.FS

const fileSchemaURL = "https://getsphericon.app/schemas/preset-1.schema.json"

// file is the decoded Preset file.
type file struct {
	Version     int            `json:"version"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Params      map[string]any `json:"params"`
	Verify      verifyConfig   `json:"verify"`
	Transform   string         `json:"transform"`
	Tests       []testCase     `json:"tests"`
}

type testCase struct {
	Name    string         `json:"name"`
	Now     string         `json:"now"`
	Params  map[string]any `json:"params"`
	Request struct {
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	} `json:"request"`
	Expect struct {
		Failures *[]DeliveryFailure `json:"failures"`
		Action   *struct {
			Kind ActionKind `json:"kind"`
			URL  string     `json:"url"`
		} `json:"action"`
		Refused string `json:"refused"`
	} `json:"expect"`
}

// Parse reads one Preset file: it must be YAML, valid against the published Preset
// JSON Schema, carry a params schema that compiles, a verify block that is coherent
// and a transform that compiles. An error names the file and the line.
func Parse(name string, data []byte, opts ...Option) (*Preset, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, parseError(name, yamlErrorLine(err), err.Error())
	}
	if root.Kind == 0 || len(root.Content) == 0 {
		return nil, parseError(name, 1, "the file is empty")
	}
	var generic any
	if err := root.Content[0].Decode(&generic); err != nil {
		return nil, parseError(name, root.Content[0].Line, err.Error())
	}
	raw, err := json.Marshal(generic)
	if err != nil {
		return nil, parseError(name, root.Content[0].Line, err.Error())
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, parseError(name, 1, err.Error())
	}
	if err := fileSchema.Validate(doc); err != nil {
		line := 1
		if ve, ok := errors.AsType[*jsonschema.ValidationError](err); ok {
			line = lineOf(root.Content[0], deepest(ve))
		}
		return nil, parseError(name, line, err.Error())
	}

	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, parseError(name, 1, err.Error())
	}
	at := func(keys ...string) int { return lineOf(root.Content[0], keys) }

	paramsDoc, err := compileParams(f.Params)
	if err != nil {
		return nil, parseError(name, at("params"), err.Error())
	}
	v, err := newVerifier(f.Verify, buildOptions(opts))
	if err != nil {
		return nil, parseError(name, at("verify"), err.Error())
	}
	code, err := compileTransform(f.Transform)
	if err != nil {
		return nil, parseError(name, at("transform"), err.Error())
	}
	for i, c := range f.Tests {
		if c.Now == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, c.Now); err != nil {
			return nil, parseError(name, at("tests", strconv.Itoa(i), "now"), "now is not an RFC 3339 time")
		}
	}
	return &Preset{
		name:        f.Name,
		description: f.Description,
		params:      f.Params,
		paramsDoc:   paramsDoc,
		verifier:    v,
		transform:   code,
		tests:       f.Tests,
	}, nil
}

func parseError(file string, line int, msg string) error {
	return fmt.Errorf("preset %s:%d: %s", file, line, msg)
}

var fileSchema = mustCompileFileSchema()

func mustCompileFileSchema() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(Schema))
	if err != nil {
		panic(fmt.Sprintf("preset: the embedded schema is not JSON: %v", err))
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	if err := c.AddResource(fileSchemaURL, doc); err != nil {
		panic(fmt.Sprintf("preset: the embedded schema: %v", err))
	}
	s, err := c.Compile(fileSchemaURL)
	if err != nil {
		panic(fmt.Sprintf("preset: the embedded schema: %v", err))
	}
	return s
}

func compileParams(params map[string]any) (*jsonschema.Schema, error) {
	doc, err := jsonValue(params)
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	const url = "params.json"
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("params: %w", err)
	}
	s, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("params: %w", err)
	}
	return s, nil
}

// deepest is the instance location of the most specific cause of a validation error.
func deepest(ve *jsonschema.ValidationError) []string {
	best := ve.InstanceLocation
	for _, c := range ve.Causes {
		if loc := deepest(c); len(loc) > len(best) || (len(loc) == len(best) && isKind(best) && !isKind(loc)) {
			best = loc
		}
	}
	return best
}

// isKind reports whether a location is the `kind` discriminator of a verify block: a
// `kind` that does not match one oneOf branch is that branch's noise, never the mistake.
func isKind(loc []string) bool { return len(loc) > 0 && loc[len(loc)-1] == "kind" }

// lineOf finds the 1-based line of the node at a path of map keys and sequence
// indexes, or of the deepest node on it that exists.
func lineOf(n *yaml.Node, keys []string) int {
	line := n.Line
	for _, k := range keys {
		var next *yaml.Node
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == k {
					next = n.Content[i+1]
					line = n.Content[i].Line
					break
				}
			}
		case yaml.SequenceNode:
			if i, err := strconv.Atoi(k); err == nil && i >= 0 && i < len(n.Content) {
				next = n.Content[i]
				line = next.Line
			}
		}
		if next == nil {
			return line
		}
		n = next
	}
	return line
}

// yamlErrorLine pulls the line out of a yaml.v3 syntax error ("yaml: line 3: ...").
func yamlErrorLine(err error) int {
	_, rest, ok := strings.Cut(err.Error(), "line ")
	if !ok {
		return 1
	}
	num, _, _ := strings.Cut(rest, ":")
	if n, err := strconv.Atoi(num); err == nil {
		return n
	}
	return 1
}

// Registry holds every Preset of an Instance: the built-in ones and the Self-hoster's.
type Registry struct {
	byName map[string]*Preset
}

// Load parses the built-in Presets and, when custom is not nil, every *.yaml and *.yml
// file at its root. It is the one entry the server uses at start: a file that fails to
// parse, to validate or to compile, or a name that collides with a built-in (or with
// another custom file), is an error naming the file.
func Load(custom fs.FS, opts ...Option) (*Registry, error) {
	r := &Registry{byName: map[string]*Preset{}}
	sub, err := fs.Sub(builtinFS, "builtin")
	if err != nil {
		return nil, err
	}
	if err := r.add(sub, "built-in ", opts); err != nil {
		return nil, err
	}
	if custom == nil {
		return r, nil
	}
	if err := r.add(custom, "", opts); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Registry) add(fsys fs.FS, label string, opts []Option) error {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("preset: read directory: %w", err)
	}
	for _, e := range entries {
		ext := path.Ext(e.Name())
		if e.IsDir() || (ext != ".yaml" && ext != ".yml") {
			continue
		}
		data, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return fmt.Errorf("preset %s%s: %w", label, e.Name(), err)
		}
		p, err := Parse(label+e.Name(), data, opts...)
		if err != nil {
			return err
		}
		if _, taken := r.byName[p.name]; taken {
			return fmt.Errorf("preset %s%s: the name %q is already taken by a built-in or another Preset", label, e.Name(), p.name)
		}
		r.byName[p.name] = p
	}
	return nil
}

// Get returns the Preset of that name.
func (r *Registry) Get(name string) (*Preset, bool) {
	p, ok := r.byName[name]
	return p, ok
}

// Names lists the Presets' names in order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.byName))
	for n := range r.byName {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// CaseResult is the outcome of one of the Preset's own tests: Err is nil when the case
// passed, and says how it disagreed otherwise.
type CaseResult struct {
	Name string
	Err  error
}

// RunTests runs each case the Preset carries: it posts the case's request with its
// params and clock and expects the Delivery failures, or the refusal, it names.
func (p *Preset) RunTests(ctx context.Context) []CaseResult {
	results := make([]CaseResult, 0, len(p.tests))
	for _, c := range p.tests {
		results = append(results, CaseResult{Name: c.Name, Err: p.forTest().runCase(ctx, c)})
	}
	return results
}

// Test runs the cases the Preset carries; the error lists every case that disagreed.
func (p *Preset) Test(ctx context.Context) error {
	var failed []error
	for _, r := range p.RunTests(ctx) {
		if r.Err != nil {
			failed = append(failed, fmt.Errorf("preset %s, case %q: %w", p.name, r.Name, r.Err))
		}
	}
	return errors.Join(failed...)
}

func (p *Preset) runCase(ctx context.Context, c testCase) error {
	now := time.Unix(0, 0)
	if c.Now != "" {
		now, _ = time.Parse(time.RFC3339, c.Now) // validated at load
	}
	header := http.Header{}
	for k, v := range c.Request.Headers {
		header.Set(k, v)
	}
	res, err := p.Handle(ctx, Request{Header: header, Body: []byte(c.Request.Body)}, Params(c.Params), now)

	if c.Expect.Failures == nil {
		want := map[string]error{
			"unauthentic":      ErrUnauthentic,
			"malformed":        ErrMalformed,
			"transform_failed": ErrTransformFailed,
		}[c.Expect.Refused]
		if !errors.Is(err, want) {
			return fmt.Errorf("expected a refusal %q, got failures %v and error %w", c.Expect.Refused, res.Failures, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("expected failures, got an error: %w", err)
	}
	if !slices.Equal(res.Failures, *c.Expect.Failures) && (len(res.Failures) != 0 || len(*c.Expect.Failures) != 0) {
		return fmt.Errorf("expected failures %v, got %v", *c.Expect.Failures, res.Failures)
	}
	switch want := c.Expect.Action; {
	case want == nil && res.Action != nil:
		return fmt.Errorf("expected no action, got %s %s", res.Action.Kind, res.Action.URL)
	case want != nil && (res.Action == nil || res.Action.Kind != want.Kind || res.Action.URL != want.URL):
		return fmt.Errorf("expected action %s %s, got %v", want.Kind, want.URL, res.Action)
	}
	return nil
}
