package convert_test

import (
	"testing"
	"time"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/patch"
)

// optNilInt mimics an ogen OptNilInt: absent, null or a value.
type optNilInt struct {
	v         int
	set, null bool
}

func (o optNilInt) Get() (int, bool) { return o.v, o.set && !o.null }
func (o optNilInt) IsSet() bool      { return o.set }

func TestPatchKeepsAnAbsentKeyClearsANullAndSetsAValue(t *testing.T) {
	assert.True(t, convert.Patch[int](optNilInt{}).IsKeep())
	assert.True(t, convert.Patch[int](optNilInt{set: true, null: true}).IsClear())
	assert.Equal(t, patch.Set(7), convert.Patch[int](optNilInt{v: 7, set: true}))
}

func TestStringPatchCoercesANamedString(t *testing.T) {
	assert.Equal(t, patch.Set("Europe/Berlin"), convert.StringPatch[zone](optZone{v: "Europe/Berlin", set: true}))
	assert.True(t, convert.StringPatch[zone](optZone{}).IsKeep())
}

func TestListPatchKeepsANilListAndSetsAnEmptyOne(t *testing.T) {
	assert.True(t, convert.ListPatch([]string(nil)).IsKeep())
	assert.Equal(t, patch.Set([]string{}), convert.ListPatch([]string{}))
}

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
func (o optZone) IsSet() bool       { return o.set }

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

// nilInt mimics an ogen NilInt64: a value, or null.
type nilInt struct {
	v    int64
	null bool
}

func (n *nilInt) SetTo(v int64) { n.v, n.null = v, false }
func (n *nilInt) SetToNull()    { n.v, n.null = 0, true }

func TestNilIsNullWithoutAValue(t *testing.T) {
	seven := int64(7)

	set := convert.Nil[nilInt](&seven)
	unset := convert.Nil[nilInt]((*int64)(nil))

	assert.Equal(t, nilInt{v: 7}, set)
	assert.Equal(t, nilInt{null: true}, unset)
}

func TestWholeSecondsDropsTheFraction(t *testing.T) {
	d := 90*time.Second + 999*time.Millisecond

	got := convert.WholeSeconds(&d)
	none := convert.WholeSeconds(nil)

	require.NotNil(t, got)
	assert.Equal(t, int64(90), *got)
	assert.Nil(t, none)
}

func TestStringPtr(t *testing.T) {
	got := convert.StringPtr[zone](optZone{v: "Europe/Berlin", set: true})
	require.NotNil(t, got)
	assert.Equal(t, "Europe/Berlin", *got)

	assert.Nil(t, convert.StringPtr[zone](optZone{}))
}
