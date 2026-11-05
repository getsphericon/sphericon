package events_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/events"
)

func TestBoundedPropertiesKeepsWhatFitsTheBoundAndDropsWhatDoesNot(t *testing.T) {
	small := map[string]any{"url": "https://x.test"}
	large := map[string]any{"pad": strings.Repeat("x", events.PropertiesBound)}

	for name, tc := range map[string]struct{ in, want map[string]any }{
		"none":      {in: nil, want: nil},
		"fits":      {in: small, want: small},
		"too large": {in: large, want: nil},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, events.BoundedProperties(tc.in))
		})
	}
}
