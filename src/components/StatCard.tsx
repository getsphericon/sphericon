import { Card, NumberFormatter, Text } from '@mantine/core'

type Figure = number | string

// A figure as a card shows it: a 0–1 ratio as a percentage with `percent`, a count as it is, and a
// string as the caller formatted it (an amount in a currency).
function figure(value: Figure, percent: boolean) {
  return percent && typeof value === 'number' ? (
    <NumberFormatter value={value * 100} suffix="%" decimalScale={1} />
  ) : (
    value
  )
}

// StatCard shows a single labelled metric. With `percent`, the value is
// rendered as a percentage (the value is a 0–1 ratio); otherwise as a count. A string value is
// already formatted by the caller (an amount in a currency) and shown as it is. A `comparison` is a
// line under the value: the figure of an earlier period, shown the way the value is, with its
// label and the change since.
// Shared by the broadcast report and the workspace analytics dashboard.
export function StatCard({
  label,
  value,
  percent = false,
  comparison,
}: {
  label: string
  value: Figure
  percent?: boolean
  comparison?: { label: string; value: Figure; change: string | null }
}) {
  return (
    <Card withBorder padding="md">
      <Text size="xs" c="dimmed" tt="uppercase">
        {label}
      </Text>
      <Text fw={700} size="xl">
        {figure(value, percent)}
      </Text>
      {comparison === undefined ? null : (
        <Text size="xs" c="dimmed">
          {comparison.label}: {figure(comparison.value, percent)}
          {comparison.change === null ? null : ` (${comparison.change})`}
        </Text>
      )}
    </Card>
  )
}
