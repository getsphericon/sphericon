package integrations

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/go-faster/jx"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/preset"
)

// The inbound side of an Integration (ADR 0037): the Preset its provider callbacks are
// read with, the Preset's params (secrets encrypted, rotating in pairs) and its own hook
// key. The hook (internal/server) opens an Integration by key with the raw client, then
// asks this module for its params; everything an API reads or writes goes through here.

// SecretMask is what a read shows in place of a secret param. Sending it back, or a
// blank, or nothing, leaves the stored secret as it is.
const SecretMask = "********"

// PresetChange is the Preset of a write: unset keeps the stored one, Name nil removes it.
type PresetChange struct {
	Set  bool
	Name *string
}

// PresetOf reads the Preset of an optional-nullable request field (the generated
// OptNilString of either API).
func PresetOf(in interface {
	Get() (string, bool)
	IsSet() bool
}) PresetChange {
	v, ok := in.Get()
	if !ok {
		return PresetChange{Set: in.IsSet()}
	}
	return PresetChange{Set: true, Name: &v}
}

// Presets is the registry of Presets the Instance offers.
func (m *Module) Presets() *preset.Registry { return m.presets }

// HookURL is where the provider posts this Integration's callbacks.
func (m *Module) HookURL(row *ent.Integration) string {
	return strings.TrimRight(m.appURL, "/") + "/hooks/" + row.IngestKey
}

// ParamsOf turns the opaque params object of a request into Go values.
func ParamsOf[M ~map[string]jx.Raw](raw M) (map[string]any, error) {
	if raw == nil {
		return nil, nil
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		var x any
		if err := json.Unmarshal(v, &x); err != nil {
			return nil, &ValidationError{Field: "params", Message: err.Error()}
		}
		out[k] = x
	}
	return out, nil
}

// RawOf renders params as the opaque object of a response.
func RawOf(values map[string]any) map[string]jx.Raw {
	out := make(map[string]jx.Raw, len(values))
	for k, v := range values {
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		out[k] = b
	}
	return out
}

// ParamsView is the read side of an Integration's params: the values as saved with each
// secret masked, and the secrets that are rotating.
type ParamsView struct {
	Values          map[string]any
	SecretsRetiring []string
}

// View opens the stored params and returns them without a secret.
func (m *Module) View(row *ent.Integration) (ParamsView, error) {
	view := ParamsView{Values: map[string]any{}, SecretsRetiring: []string{}}
	if row.Preset == nil || row.ParamsEncrypted == "" {
		return view, nil
	}
	p, ok := m.presets.Get(*row.Preset)
	if !ok {
		return view, nil
	}
	stored, err := m.openParams(row)
	if err != nil {
		return ParamsView{}, err
	}
	secret := secretSet(p)
	for name, v := range stored {
		if !secret[name] {
			view.Values[name] = v
			continue
		}
		view.Values[name] = SecretMask
		if list, ok := v.([]any); ok && len(list) > 1 {
			view.SecretsRetiring = append(view.SecretsRetiring, name)
		}
	}
	slices.Sort(view.SecretsRetiring)
	return view, nil
}

// Params opens the stored params for the engine: a rotating secret is a list, the
// current value first.
func (m *Module) Params(row *ent.Integration) (preset.Params, error) {
	if row.ParamsEncrypted == "" {
		return preset.Params{}, nil
	}
	stored, err := m.openParams(row)
	return preset.Params(stored), err
}

func (m *Module) openParams(row *ent.Integration) (map[string]any, error) {
	plaintext, err := m.cipher.Decrypt(row.ParamsEncrypted)
	if err != nil {
		return nil, err
	}
	stored := map[string]any{}
	if err := json.Unmarshal(plaintext, &stored); err != nil {
		return nil, err
	}
	return stored, nil
}

// inbound is what an Integration write leaves behind on the inbound side. apply is false
// when the write does not touch it (the row keeps its Preset and params as stored); true
// with a nil sealed clears both.
type inbound struct {
	apply  bool
	preset *string
	sealed *string // nil: no params
}

// inboundChange is the inbound part of a write: the Preset, the params to merge over the
// stored ones (nil: unchanged) and whether the retiring secret of every rotating pair is
// dropped.
type inboundChange struct {
	preset        PresetChange
	params        map[string]any
	retireSecrets bool
}

// sealInbound validates a write's Preset and params against the stored ones and returns
// what to store. Changing the Preset carries nothing over: no secret of the old one is
// valid for the new, so its params must be given in full. Within one Preset a secret that
// is omitted, blank or masked keeps its stored value, and a new value rotates it: the old
// current value stays accepted, as the retiring one, until retired.
func (m *Module) sealInbound(row *ent.Integration, in inboundChange) (inbound, error) {
	var prevName *string
	if row != nil {
		prevName = row.Preset
	}
	name := prevName
	if in.preset.Set {
		name = in.preset.Name
	}
	changed := !samePreset(prevName, name)
	if row != nil && !changed && in.params == nil && !in.retireSecrets {
		return inbound{}, nil
	}
	if name == nil {
		if len(in.params) > 0 {
			return inbound{}, &ValidationError{Field: "params", Message: i18n.T("errors.preset_params_without_preset", nil)}
		}
		return inbound{apply: row != nil && changed}, nil
	}
	p, ok := m.presets.Get(*name)
	if !ok {
		return inbound{}, &ValidationError{Field: "preset", Message: i18n.T("errors.preset_unknown", map[string]any{"Name": *name})}
	}

	var prev map[string]any
	if row != nil && !changed && row.ParamsEncrypted != "" {
		var err error
		if prev, err = m.openParams(row); err != nil {
			return inbound{}, err
		}
	}
	stored, plain := mergeParams(p, prev, in.params)
	if err := p.ValidateParams(plain); err != nil {
		return inbound{}, &ValidationError{Field: "params", Message: i18n.T("errors.preset_params_invalid", map[string]any{"Detail": err.Error()})}
	}
	if in.retireSecrets {
		for _, s := range p.SecretParams() {
			if list, ok := stored[s].([]any); ok && len(list) > 0 {
				stored[s] = list[0]
			}
		}
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return inbound{}, err
	}
	sealed, err := m.cipher.Encrypt(raw)
	if err != nil {
		return inbound{}, err
	}
	return inbound{apply: true, preset: name, sealed: &sealed}, nil
}

// mergeParams builds the stored params (a rotating secret is [current, retiring]) and
// the plain ones the schema validates (every secret a single string).
func mergeParams(p *preset.Preset, prev, in map[string]any) (stored, plain map[string]any) {
	secret := secretSet(p)
	stored, plain = map[string]any{}, map[string]any{}
	base := in
	if base == nil {
		base = prev
	}
	for k, v := range base {
		if !secret[k] {
			stored[k], plain[k] = v, v
		}
	}
	for name := range secret {
		incoming, _ := in[name].(string)
		_, given := in[name]
		old := currentSecret(prev[name])
		switch {
		case given && incoming != "" && incoming != SecretMask:
			plain[name] = incoming
			switch {
			case old != "" && old != incoming:
				stored[name] = []any{incoming, old}
			case old == incoming:
				stored[name] = prev[name]
			default:
				stored[name] = incoming
			}
		case given && incoming == "" && !isJSONString(in[name]):
			// The write sent a non-string (a number, null, a list) for a secret; "" above is
			// also what a non-string reads as. Pass it through so the schema refuses it,
			// rather than treating it as "keep the stored secret".
			plain[name] = in[name]
			stored[name] = in[name]
		default:
			if v, ok := prev[name]; ok {
				stored[name] = v
				plain[name] = old
			}
		}
	}
	return stored, plain
}

// currentSecret is the value a stored secret is accepted as today: a plain secret itself,
// a rotating pair's first (current) element, "" for anything else.
func currentSecret(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		if len(x) > 0 {
			s, _ := x[0].(string)
			return s
		}
	}
	return ""
}

func isJSONString(v any) bool { _, ok := v.(string); return ok }

func secretSet(p *preset.Preset) map[string]bool {
	out := map[string]bool{}
	for _, s := range p.SecretParams() {
		out[s] = true
	}
	return out
}

func samePreset(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// RegenerateKey gives the Integration a new hook key: the old hook URL stops working at
// once. The write is audited like any other edit of the Integration; the key itself never
// reaches the entry.
func (m *Module) RegenerateKey(ctx context.Context, s *ent.Scoped, id int64) (*ent.Integration, error) {
	key, err := credentials.GenerateIngestKey()
	if err != nil {
		return nil, err
	}
	var row *ent.Integration
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		var err error
		row, err = ts.Integration().UpdateOneID(id).SetIngestKey(key).Save(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}
