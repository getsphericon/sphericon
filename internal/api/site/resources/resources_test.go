package resources_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/api/site/resources"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

func TestBroadcastStatsRates(t *testing.T) {
	// 100 targeted, 80 delivered, 40 opened, 20 clicked, 5 unsubscribed, 20 failed.
	got := (&resources.ConverterImpl{}).BroadcastStatsToResource(broadcasts.Stats{
		Channel:      channels.Email,
		Recipients:   100,
		Sent:         80,
		Opened:       40,
		Clicked:      20,
		Unsubscribed: 5,
		Failed:       20,
	}.Figures())

	cases := []struct {
		name string
		got  float32
		want float32
	}{
		{"deliveryRate", got.DeliveryRate.Value, 0.80},       // 80/100
		{"openRate", got.OpenRate.Value, 0.50},               // 40/80
		{"clickRate", got.ClickRate, 0.25},                   // 20/80
		{"clickToOpenRate", got.ClickToOpenRate.Value, 0.50}, // 20/40
		{"unsubscribeRate", got.UnsubscribeRate, 0.0625},     // 5/80
		{"failureRate", got.FailureRate, 0.20},               // 20/100
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
	got := (&resources.ConverterImpl{}).BroadcastStatsToResource(broadcasts.Stats{}.Figures())
	rates := map[string]float32{
		"deliveryRate":    got.DeliveryRate.Value,
		"openRate":        got.OpenRate.Value,
		"clickRate":       got.ClickRate,
		"clickToOpenRate": got.ClickToOpenRate.Value,
		"unsubscribeRate": got.UnsubscribeRate,
		"failureRate":     got.FailureRate,
	}
	for name, v := range rates {
		assert.InDeltaf(t, 0, v, 1e-9, "%s = %v, want 0", name, v)
	}

	// Delivered but nobody opened: openRate defined (0), clickToOpenRate guarded (0).
	got = (&resources.ConverterImpl{}).BroadcastStatsToResource(broadcasts.Stats{Recipients: 10, Sent: 10}.Figures())
	assert.InDeltaf(t, 1, got.DeliveryRate.Value, 1e-9, "deliveryRate = %v, want 1", got.DeliveryRate)
	assert.InDelta(t, 0, got.OpenRate.Value, 1e-9)
	assert.InDelta(t, 0, got.ClickToOpenRate.Value, 1e-9)
}

// Telegram reports no opens and no delivery results, so those figures and the rates built on them
// are null, not zero; what it does report passes through.
func TestBroadcastStatsTelegramReportsNoOpensOrDelivery(t *testing.T) {
	got := (&resources.ConverterImpl{}).BroadcastStatsToResource(broadcasts.Stats{
		Channel: channels.Telegram, Recipients: 10, Sent: 6, Skipped: 3, Failed: 1, Clicked: 2,
	}.Figures())

	assert.True(t, got.OpenedCount.Null)
	assert.True(t, got.DeliveredCount.Null)
	assert.True(t, got.UndeliveredCount.Null)
	assert.True(t, got.BouncedCount.Null)
	assert.True(t, got.ComplainedCount.Null)
	assert.True(t, got.CapturedCount.Null)
	assert.True(t, got.DeliveryRate.Null)
	assert.True(t, got.OpenRate.Null)
	assert.True(t, got.ClickToOpenRate.Null)
	assert.EqualValues(t, 6, got.SentCount)
	assert.EqualValues(t, 3, got.SkippedCount)
	assert.EqualValues(t, 2, got.ClickedCount)
	assert.InDelta(t, 2.0/6, got.ClickRate, 1e-6)
}

// SMS reports delivered and undelivered texts, and no opens, bounces or complaints; email reports no
// undelivered: it says bounced.
func TestBroadcastStatsSMSReportsDeliveredAndUndelivered(t *testing.T) {
	sms := (&resources.ConverterImpl{}).BroadcastStatsToResource(broadcasts.Stats{
		Channel: channels.SMS, Recipients: 10, Sent: 9, Delivered: 6, Undelivered: 2,
	}.Figures())
	email := (&resources.ConverterImpl{}).BroadcastStatsToResource(broadcasts.Stats{
		Channel: channels.Email, Recipients: 10, Sent: 9, Delivered: 6, Bounced: 1,
	}.Figures())

	assert.EqualValues(t, 6, sms.DeliveredCount.Value)
	assert.EqualValues(t, 2, sms.UndeliveredCount.Value)
	assert.False(t, sms.UndeliveredCount.Null)
	assert.True(t, sms.BouncedCount.Null)
	assert.True(t, sms.ComplainedCount.Null)
	assert.True(t, sms.OpenedCount.Null)
	assert.True(t, sms.DeliveryRate.Null)
	assert.True(t, sms.CapturedCount.Null)
	assert.EqualValues(t, 6, email.DeliveredCount.Value)
	assert.EqualValues(t, 1, email.BouncedCount.Value)
	assert.True(t, email.UndeliveredCount.Null)
}
