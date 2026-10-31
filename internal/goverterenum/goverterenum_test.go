package goverterenum_test

import (
	"go/types"
	"testing"

	"github.com/jmattheis/goverter/enum"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/goverterenum"
)

func enumOf(name string, members map[string]any) enum.Enum {
	obj := types.NewTypeName(0, types.NewPackage("example.com/"+name, name), name, nil)
	return enum.Enum{Type: types.NewNamed(obj, types.Typ[types.String], nil), Members: members}
}

func TestByValueMatchesMembersOfTheSameValue(t *testing.T) {
	got, err := goverterenum.ByValue(enum.TransformContext{
		Source: enumOf("Status", map[string]any{"StatusSent": "sent", "DefaultStatus": "draft", "StatusDraft": "draft", "StatusLost": "lost"}),
		Target: enumOf("SiteStatus", map[string]any{"SiteStatusSent": "sent", "SiteStatusDraft": "draft"}),
	})

	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"StatusSent":    "SiteStatusSent",
		"StatusDraft":   "SiteStatusDraft",
		"DefaultStatus": "SiteStatusDraft",
	}, got, "a member with no counterpart is left for goverter to refuse")
}

func TestByValueRefusesATargetValueNamedTwice(t *testing.T) {
	_, err := goverterenum.ByValue(enum.TransformContext{
		Source: enumOf("Status", map[string]any{"StatusDraft": "draft"}),
		Target: enumOf("SiteStatus", map[string]any{"SiteStatusDraft": "draft", "SiteStatusDefault": "draft"}),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "SiteStatusDefault, SiteStatusDraft")
}
