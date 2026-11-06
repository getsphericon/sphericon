package preset

import (
	"errors"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// ParamErrors are the refusals of ValidateParams by the param each is about: where the failing
// value sits in the params, or the param a required or not-allowed failure names. A failure of
// the params as a whole is under "". Only the deepest failures are kept, the ones that say what to
// fix; an error that is not a schema refusal is under "" as it is.
func ParamErrors(err error) map[string][]string {
	ve, ok := errors.AsType[*jsonschema.ValidationError](err)
	if !ok {
		return map[string][]string{"": {err.Error()}}
	}
	out := map[string][]string{}
	printer := message.NewPrinter(language.English)
	var walk func(*jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) > 0 {
			for _, c := range e.Causes {
				walk(c)
			}
			return
		}
		text := e.ErrorKind.LocalizedString(printer)
		for _, name := range paramsOf(e) {
			out[name] = append(out[name], text)
		}
	}
	walk(ve)
	return out
}

// paramsOf names the params a leaf failure is about.
func paramsOf(e *jsonschema.ValidationError) []string {
	if len(e.InstanceLocation) > 0 {
		return []string{e.InstanceLocation[0]}
	}
	switch k := e.ErrorKind.(type) {
	case *kind.Required:
		return k.Missing
	case *kind.AdditionalProperties:
		return k.Properties
	}
	return []string{""}
}
