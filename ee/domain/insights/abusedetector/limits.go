// Package abusedetector is the Enterprise Abuse detector (ADR 0026, GLOSSARY "Abuse detector"):
// its limits, its reading of a Workspace's standing against them (the Complaint rate and Bounce
// rate per Sending domain over the trailing 24 hours, the rates of internal/domain/sending/reputation, no new
// math), the console's review filters and the detector's hourly pass (Detector, detector.go, ADR
// 0112). The lower tier only warns the owners; the higher one freezes the Workspace. The Watch
// only reads the standing, which the Staff console and the dashboard read through it too; the
// Detector acts on it.
//
// Governed by ee/LICENSE, not the AGPL.
package abusedetector

import (
	"fmt"
	"math"
	"time"

	"github.com/getsphericon/sphericon/internal/domain/insights/abusewarning"
	"github.com/getsphericon/sphericon/internal/domain/sending/reputation"
)

// Window is the trailing span the detector judges (ADR 0026).
const Window = 24 * time.Hour

// Tier is how far a Workspace is over the limits.
type Tier int

const (
	TierNone Tier = iota
	// TierWarn only warns the owners.
	TierWarn
	// TierFreeze suspends the Workspace.
	TierFreeze
)

// Metric is the rate that breached. It is core's, which the /site warning carries.
type Metric = abusewarning.Metric

const (
	Complaint = abusewarning.MetricComplaint
	Bounce    = abusewarning.MetricBounce
)

// Limits are the detector's thresholds in basis points (1 bp = 0.01%), so a comparison is
// exact integer arithmetic. The zero value is DefaultLimits.
type Limits struct {
	// VolumeFloor is the fewest sends in the window, per Sending domain, for it to be judged.
	VolumeFloor                       int
	WarnComplaintBP, WarnBounceBP     int
	FreezeComplaintBP, FreezeBounceBP int
}

// DefaultLimits are ADR 0026's: floor 1000 sent; warn at 0.1% complaints or 5% bounces;
// freeze at 0.3% or 10%.
func DefaultLimits() Limits {
	return Limits{VolumeFloor: 1000, WarnComplaintBP: 10, WarnBounceBP: 500, FreezeComplaintBP: 30, FreezeBounceBP: 1000}
}

// LimitsFromPercent builds Limits from the configured percentages.
func LimitsFromPercent(floor int, warnComplaint, warnBounce, freezeComplaint, freezeBounce float64) Limits {
	bp := func(p float64) int { return int(math.Round(p * 100)) }
	return Limits{
		VolumeFloor:     floor,
		WarnComplaintBP: bp(warnComplaint), WarnBounceBP: bp(warnBounce),
		FreezeComplaintBP: bp(freezeComplaint), FreezeBounceBP: bp(freezeBounce),
	}
}

func (l Limits) orDefault() Limits {
	if l == (Limits{}) {
		return DefaultLimits()
	}
	return l
}

// Breach is the worst standing of a Workspace's Sending domains. The zero value (TierNone)
// means nothing is over a limit.
type Breach struct {
	Tier   Tier
	Domain string
	Metric Metric
	// Numerator over Denominator is the rate: complaints over delivered, or hard bounces
	// over sent.
	Numerator, Denominator int
	// LimitBP is the limit of the tier that was reached.
	LimitBP int
}

// Rate is the breaching rate, 0–1.
func (b Breach) Rate() float64 {
	if b.Denominator == 0 {
		return 0
	}
	return float64(b.Numerator) / float64(b.Denominator)
}

// Describe is the breach as a sentence, e.g. for a suspension reason.
func (b Breach) Describe() string {
	basis := "delivered"
	if b.Metric == Bounce {
		basis = "sent"
	}
	return fmt.Sprintf("%s rate %s (%d of %d %s) on %s in the last 24 hours, at or over the %s limit",
		b.Metric, Percent(b.Rate()), b.Numerator, b.Denominator, basis, b.Domain, Percent(float64(b.LimitBP)/10000))
}

// Percent renders a 0–1 rate as a percentage without trailing zeros.
func Percent(r float64) string { return fmt.Sprintf("%.4g%%", r*100) }

// reached reports whether num/den is at or over bp basis points, exactly.
func reached(num, den, bp int) bool { return den > 0 && num*10000 >= bp*den }

// Classify is the one answer to "what tier is this Workspace in", shared by the detector,
// the console and /site. A Sending domain below the volume floor is not judged; among
// breaches the highest tier wins, the first domain and the complaint rate on a tie.
func (l Limits) Classify(rates []reputation.DomainRates) Breach {
	l = l.orDefault()
	var worst Breach
	consider := func(tier Tier, b Breach) {
		if tier > worst.Tier {
			b.Tier = tier
			worst = b
		}
	}
	for _, r := range rates {
		if r.Bounce.Denominator < l.VolumeFloor {
			continue
		}
		for _, m := range []struct {
			metric       Metric
			rate         reputation.Rate
			warn, freeze int
		}{
			{Complaint, r.Complaint, l.WarnComplaintBP, l.FreezeComplaintBP},
			{Bounce, r.Bounce, l.WarnBounceBP, l.FreezeBounceBP},
		} {
			b := Breach{Domain: r.Domain.Domain, Metric: m.metric, Numerator: m.rate.Numerator, Denominator: m.rate.Denominator}
			switch {
			case reached(m.rate.Numerator, m.rate.Denominator, m.freeze):
				b.LimitBP = m.freeze
				consider(TierFreeze, b)
			case reached(m.rate.Numerator, m.rate.Denominator, m.warn):
				b.LimitBP = m.warn
				consider(TierWarn, b)
			}
		}
	}
	return worst
}
