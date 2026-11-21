---
status: accepted
build: none
terms: [Conversion window]
---

# A Split's paths are compared with its first by a test that may be read at any time

> Amends [[0124-an-automation-chooses-a-path-by-condition-by-weight-or-by-an-awaited-event]] (the
> report of a Split says whether a path differs from the first, beside its figures).

## Context

A Split tests one path against another (ADR 0124), and its report gives each path's Enrollments and
goals, but nothing says whether a difference is more than noise. The author decides which paths to
drop from those figures alone. A Broadcast's Split test reads its significance once, when its fixed
test time ends (ADR 0152). A Split has no end: it runs for as long as the Automation does, and its
report is read whenever someone opens it. Reading a fixed-sample test again and again and stopping
at the first significant result multiplies false positives (one worked example stopped at the first
5% result and was wrong 26% of the time); the manuals agents load say to fix the sample size first or
use a test built for peeking. Klaviyo's flow tests call a result significant when the leader's and the
runner-up's credible intervals overlap by less than 10%, read at any time; Braze's Experiment Paths
count conversions within 1 to 30 days of entering the step, and warn that delays inside a path use
that window up.

## Decision

- **The goal rate of a path** is the share of its Enrollments that reached the goal within the
  Split's **Conversion window** of passing the Split, counted over the Enrollments that passed it at
  least that long ago. An Enrollment still inside the window is not counted yet, so a path with longer
  delays is not read as worse; an Enrollment that exited or failed inside it counts as not converted.
  It is the one figure an empty path, a holdout, has.
- **The Conversion window** is a setting of the Split, 1 to 30 days, 7 by default, part of its
  version. The Attribution window is not reused: it credits an Order to a click, a different question.
- **A content variant** (each path holds exactly one send Step, all on one Channel) is compared on the
  click rate and the Revenue per recipient of those Steps too, as a Broadcast's Variants are (ADR 0152,
  ADR 0191). Paths with different numbers of messages are not compared on message figures.
- **The test may be read at any time**: an always-valid confidence sequence, the mixture sequential
  probability ratio test (Johari et al.), on the difference of two proportions or two means, at 95%.
  Its interval holds however often the report is read, so the report never shows a significance that
  peeking produced. A Broadcast's Split test keeps its fixed-time tests, read once at its end.
- **Each path is compared with the first path**, the control, which the author orders first (a holdout
  goes there). With more than two paths the 5% is divided by the number of comparisons (Bonferroni).
- **Within one version**: a comparison reads the Enrollments of one version, whose paths and weights
  are the same for all of them, the live version by default and an earlier one on request. The figures
  summed across versions are still shown, and no significance is read from them.
- **A minimum before any reading**: 100 counted Enrollments and 10 conversions per path; below it the
  report says it is collecting, with the estimate and its interval.
- **What the report gives**, per path against the control and per figure: the estimate of the
  difference, its interval, and whether it is significant, on `/api`, `/site` and MCP. The console marks
  a significant path.
- **No path is picked for the author** (ADR 0124 stands): no test stops itself and no path is dropped
  automatically. The author publishes a version without the losing paths.
- **A Branch gets no significance**: its paths are chosen by a Condition, not at random, so a
  difference between them is no effect of the path.

## Consequences

- The goal rate needs each path's Enrollments and goals from the report of a fork; the content
  variant's figures need a Step's figures by Step id and its Revenue per recipient.
- The test is implemented from the paper's closed form over a standard distributions package; a
  simulation test keeps its false-positive rate under continuous reading at or below 5%.
- The readout is one contract model for every figure, drawn by one console component.

## Considered options

- **A fixed-sample test read whenever the report is opened**: the peeking the manuals warn against.
- **A sample size the author sets, read once when it is reached**: every Split would need a size
  before anyone knows its baseline, and the result would be frozen while the Split keeps running.
- **Klaviyo's credible-interval overlap**: as exposed to peeking as a fixed-sample test.
- **Comparing every pair, or each path with the leader**: more comparisons, and the leader changes
  under noise; a control is what a holdout is for.
- **Figures summed across versions**: a change of weights or content halfway mixes two experiments.
- **Picking the path automatically**: ADR 0124 keeps that decision with the author.
