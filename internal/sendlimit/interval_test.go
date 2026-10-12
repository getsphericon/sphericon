package sendlimit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/sendlimit"
)

func ptr(n int) *int { return &n }

func TestIntervalIsOneOverTheEffectiveRate(t *testing.T) {
	cases := []struct {
		name string
		l    sendlimit.Limits
		want time.Duration
	}{
		{"unlimited", sendlimit.Limits{}, 0},
		{"per second", sendlimit.Limits{PerSecond: ptr(14)}, time.Second / 14},
		{"per day only", sendlimit.Limits{PerDay: ptr(86400)}, time.Second},
		{"the daily ceiling is slower", sendlimit.Limits{PerSecond: ptr(14), PerDay: ptr(864)}, 100 * time.Second},
		{"the per-second ceiling is slower", sendlimit.Limits{PerSecond: ptr(1), PerDay: ptr(1_000_000)}, time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { assert.Equal(t, c.want, c.l.Interval()) })
	}
}
