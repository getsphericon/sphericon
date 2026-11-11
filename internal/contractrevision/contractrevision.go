// Package contractrevision is the contract check of ADR 0119: an update of a resource that carries
// a `revision` names the revision it was made on. It reads a generated OpenAPI document and reports
// every PUT or PATCH whose answer is a resource with `revision` and whose body does not require it,
// on any surface, so an update cannot be added, or a resource given a revision, without it.
package contractrevision

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// field is the property the check is about.
const field = "revision"

type schema struct {
	Ref        string             `json:"$ref"`
	Properties map[string]*schema `json:"properties"`
	Required   []string           `json:"required"`
}

type media struct {
	Schema *schema `json:"schema"`
}

type operation struct {
	RequestBody *struct {
		Content map[string]media `json:"content"`
	} `json:"requestBody"`
	Responses map[string]struct {
		Content map[string]media `json:"content"`
	} `json:"responses"`
}

type document struct {
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]*schema `json:"schemas"`
	} `json:"components"`
}

// Report is what Check found in one document: the updates whose answer carries `revision`
// ("PUT /path"), and those of them whose body does not require it, each sorted.
type Report struct {
	Examined []string
	Missing  []string
}

// Check reads doc and reports its updates of a resource with a revision.
func Check(doc []byte) (Report, error) {
	var d document
	if err := json.Unmarshal(doc, &d); err != nil {
		return Report{}, fmt.Errorf("read the OpenAPI document: %w", err)
	}
	resolve := func(s *schema) *schema {
		for s != nil && s.Ref != "" {
			s = d.Components.Schemas[strings.TrimPrefix(s.Ref, "#/components/schemas/")]
		}
		return s
	}
	var r Report
	for path, ops := range d.Paths {
		for method, raw := range ops {
			if method != "put" && method != "patch" {
				continue
			}
			var op operation
			if err := json.Unmarshal(raw, &op); err != nil {
				return Report{}, fmt.Errorf("read %s %s: %w", method, path, err)
			}
			answer := resolve(op.Responses["200"].Content["application/json"].Schema)
			if answer == nil || answer.Properties[field] == nil {
				continue
			}
			name := strings.ToUpper(method) + " " + path
			r.Examined = append(r.Examined, name)
			var body *schema
			if op.RequestBody != nil {
				body = resolve(op.RequestBody.Content["application/json"].Schema)
			}
			if body == nil || !slices.Contains(body.Required, field) {
				r.Missing = append(r.Missing, name)
			}
		}
	}
	sort.Strings(r.Examined)
	sort.Strings(r.Missing)
	return r, nil
}
