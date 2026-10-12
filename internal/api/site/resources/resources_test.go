package resources

import (
	"testing"

	"github.com/getsphericon/sphericon/ent"
)

func TestBroadcastStatsRates(t *testing.T) {
	// 100 targeted, 80 delivered, 40 opened, 20 clicked, 5 unsubscribed, 20 failed.
	b := ent.Broadcast{
		RecipientsTotal:   100,
		SentCount:         80,
		OpenedCount:       40,
		ClickedCount:      20,
		UnsubscribedCount: 5,
		FailedCount:       20,
	}
	got := broadcastStats(b)

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
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}

	// Counters pass through unchanged.
	if got.RecipientsTotal != 100 || got.SentCount != 80 || got.FailedCount != 20 {
		t.Errorf("counters not copied: %+v", got)
	}
}

func TestBroadcastStatsZeroDenominators(t *testing.T) {
	// An all-zero broadcast (never sent): every rate must be 0, no NaN/Inf.
	got := broadcastStats(ent.Broadcast{})
	rates := map[string]float32{
		"deliveryRate":    got.DeliveryRate,
		"openRate":        got.OpenRate,
		"clickRate":       got.ClickRate,
		"clickToOpenRate": got.ClickToOpenRate,
		"unsubscribeRate": got.UnsubscribeRate,
		"failureRate":     got.FailureRate,
	}
	for name, v := range rates {
		if v != 0 {
			t.Errorf("%s = %v, want 0", name, v)
		}
	}

	// Delivered but nobody opened: openRate defined (0), clickToOpenRate guarded (0).
	got = broadcastStats(ent.Broadcast{RecipientsTotal: 10, SentCount: 10})
	if got.DeliveryRate != 1 {
		t.Errorf("deliveryRate = %v, want 1", got.DeliveryRate)
	}
	if got.OpenRate != 0 || got.ClickToOpenRate != 0 {
		t.Errorf("openRate=%v clickToOpenRate=%v, want 0/0", got.OpenRate, got.ClickToOpenRate)
	}
}
