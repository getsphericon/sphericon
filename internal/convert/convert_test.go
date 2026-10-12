package convert_test

import (
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/convert"
)

// optInt mimics an ogen OptInt: Get reports the value and whether it is set.
type optInt struct {
	v   int
	set bool
}

func (o optInt) Get() (int, bool) { return o.v, o.set }

type zone string

type optZone struct {
	v   zone
	set bool
}

func (o optZone) Get() (zone, bool) { return o.v, o.set }

func TestRawMapDecodesValues(t *testing.T) {
	type rawRecord map[string]jx.Raw
	got := convert.RawMap(rawRecord{
		"n":      jx.Raw(`42`),
		"b":      jx.Raw(`true`),
		"s":      jx.Raw(`"hi"`),
		"nested": jx.Raw(`{"a":[1,2]}`),
		"null":   jx.Raw(`null`),
	})

	assert.Equal(t, map[string]any{
		"n":      float64(42),
		"b":      true,
		"s":      "hi",
		"nested": map[string]any{"a": []any{float64(1), float64(2)}},
		"null":   nil,
	}, got)
}

func TestRawMapNilAndInvalid(t *testing.T) {
	assert.Nil(t, convert.RawMap(map[string]jx.Raw(nil)))

	got := convert.RawMap(map[string]jx.Raw{"ok": jx.Raw(`1`), "broken": jx.Raw(`{nope`)})
	assert.Equal(t, map[string]any{"ok": float64(1)}, got, "undecodable values are dropped")

	assert.Equal(t, map[string]any{}, convert.RawMap(map[string]jx.Raw{}), "empty stays an empty, non-nil map")
}

func TestPtr(t *testing.T) {
	got := convert.Ptr[int](optInt{v: 7, set: true})
	require.NotNil(t, got)
	assert.Equal(t, 7, *got)

	assert.Nil(t, convert.Ptr[int](optInt{v: 7, set: false}))
}

func TestStringPtr(t *testing.T) {
	got := convert.StringPtr[zone](optZone{v: "Europe/Berlin", set: true})
	require.NotNil(t, got)
	assert.Equal(t, "Europe/Berlin", *got)

	assert.Nil(t, convert.StringPtr[zone](optZone{}))
}
