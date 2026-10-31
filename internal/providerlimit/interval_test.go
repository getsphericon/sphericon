package providerlimit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/providerlimit"
)

func TestIntervalIsOneOverTheEffectiveRate(t *testing.T) {
	cases := []struct {
		name string
		l    providerlimit.Limits
		want time.Duration
	}{
		{"unlimited", providerlimit.Limits{}, 0},
		{"per second", providerlimit.Limits{PerSecond: new(14)}, time.Second / 14},
		{"per day only", providerlimit.Limits{PerDay: new(86400)}, time.Second},
		{"the daily ceiling is slower", providerlimit.Limits{PerSecond: new(14), PerDay: new(864)}, 100 * time.Second},
		{"the per-second ceiling is slower", providerlimit.Limits{PerSecond: new(1), PerDay: new(1_000_000)}, time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { assert.Equal(t, c.want, c.l.Interval()) })
	}
}
