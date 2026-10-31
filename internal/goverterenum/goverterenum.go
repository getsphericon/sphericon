// Package goverterenum holds the enum transformer our goverter build adds (cmd/goverter): the
// converters match enum members by value, because ent, the modules and the ogen contracts name
// the same value differently (broadcast.StatusSent, siteapi.SiteBroadcastStatusSent).
package goverterenum

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jmattheis/goverter/enum"
)

// Name is the transformer's id in `goverter:enum:transform value`.
const Name = "value"

// ByValue maps each source member to the target member that has the same value. A source member
// with no such target is left out, so goverter refuses it until a method maps it with enum:map.
// Two target members with one value (a type's Default constant beside its own) are a choice the
// transformer does not make: it names the type, and the method maps the member itself.
func ByValue(ctx enum.TransformContext) (map[string]string, error) {
	byValue := map[any][]string{}
	for name, value := range ctx.Target.Members {
		byValue[value] = append(byValue[value], name)
	}
	out := map[string]string{}
	for name, value := range ctx.Source.Members {
		targets := byValue[value]
		switch len(targets) {
		case 0:
		case 1:
			out[name] = targets[0]
		default:
			slices.Sort(targets)
			return nil, fmt.Errorf("%s: %s are all %v; map %s with enum:map",
				ctx.Target.Type.Obj().Name(), strings.Join(targets, ", "), value, name)
		}
	}
	return out, nil
}
