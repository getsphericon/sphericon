import { Card, Text } from '@mantine/core'

import { useFormat } from '../hooks/useFormat.ts'

type Figure = number | string

type Format = Pick<ReturnType<typeof useFormat>, 'formatNumber' | 'formatPercent'>

// A figure as a card shows it, in the User's Regional format: a 0–1 ratio as a percentage with
// `percent`, a count as a number, and a string as the caller formatted it (an amount in a currency).
function figure(value: Figure, percent: boolean, { formatNumber, formatPercent }: Format) {
  if (typeof value !== 'number') {
    return value
  }
  return percent ? formatPercent(value) : formatNumber(value)
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
  const format = useFormat()
  return (
    <Card withBorder padding="md">
      <Text size="xs" c="dimmed" tt="uppercase">
        {label}
      </Text>
      <Text fw={700} size="xl">
        {figure(value, percent, format)}
      </Text>
      {comparison === undefined ? null : (
        <Text size="xs" c="dimmed">
          {comparison.label}: {figure(comparison.value, percent, format)}
          {comparison.change === null ? null : ` (${comparison.change})`}
        </Text>
      )}
    </Card>
  )
}
