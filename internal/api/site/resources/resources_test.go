package resources_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/api/site/resources"
	"github.com/getsphericon/sphericon/internal/broadcasts"
)

func TestBroadcastStatsRates(t *testing.T) {
	// 100 targeted, 80 delivered, 40 opened, 20 clicked, 5 unsubscribed, 20 failed.
	got := resources.BroadcastStats(broadcasts.Stats{
		Recipients:   100,
		Sent:         80,
		Opened:       40,
		Clicked:      20,
		Unsubscribed: 5,
		Failed:       20,
	})

	cases := []struct {
		name string
		got  float32
		want float32
	}{
		{"deliveryRate", got.DeliveryRate, 0.80},         // 80/100
		{"openRate", got.OpenRate, 0.50},                 // 40/80
		{"clickRate", got.ClickRate, 0.25},               // 20/80
		{"clickToOpenRate", got.ClickToOpenRate, 0.50},   // 20/40
		{"unsubscribeRate", got.UnsubscribeRate, 0.0625}, // 5/80
		{"failureRate", got.FailureRate, 0.20},           // 20/100
	}
	for _, c := range cases {
		assert.InDeltaf(t, c.want, c.got, 1e-9, "%s = %v, want %v", c.name, c.got, c.want)
	}

	// Figures pass through unchanged.
	assert.EqualValues(t, 100, got.RecipientsTotal)
	assert.EqualValues(t, 80, got.SentCount)
	assert.EqualValues(t, 20, got.FailedCount)
}

func TestBroadcastStatsZeroDenominators(t *testing.T) {
	// An all-zero broadcast (never sent): every rate must be 0, no NaN/Inf.
	got := resources.BroadcastStats(broadcasts.Stats{})
	rates := map[string]float32{
		"deliveryRate":    got.DeliveryRate,
		"openRate":        got.OpenRate,
		"clickRate":       got.ClickRate,
		"clickToOpenRate": got.ClickToOpenRate,
		"unsubscribeRate": got.UnsubscribeRate,
		"failureRate":     got.FailureRate,
	}
	for name, v := range rates {
		assert.InDeltaf(t, 0, v, 1e-9, "%s = %v, want 0", name, v)
	}

	// Delivered but nobody opened: openRate defined (0), clickToOpenRate guarded (0).
	got = resources.BroadcastStats(broadcasts.Stats{Recipients: 10, Sent: 10})
	assert.InDeltaf(t, 1, got.DeliveryRate, 1e-9, "deliveryRate = %v, want 1", got.DeliveryRate)
	assert.InDelta(t, 0, got.OpenRate, 1e-9)
	assert.InDelta(t, 0, got.ClickToOpenRate, 1e-9)
}
