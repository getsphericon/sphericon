import { Card, NumberFormatter, Text } from '@mantine/core'

// StatCard shows a single labelled metric. With `percent`, the value is
// rendered as a percentage (the value is a 0–1 ratio); otherwise as a count. A string value is
// already formatted by the caller (an amount in a currency) and shown as it is.
// Shared by the broadcast report and the workspace analytics dashboard.
export function StatCard({
  label,
  value,
  percent = false,
}: {
  label: string
  value: number | string
  percent?: boolean
}) {
  return (
    <Card withBorder padding="md">
      <Text size="xs" c="dimmed" tt="uppercase">
        {label}
      </Text>
      <Text fw={700} size="xl">
        {percent && typeof value === 'number' ? (
          <NumberFormatter value={value * 100} suffix="%" decimalScale={1} />
        ) : (
          value
        )}
      </Text>
    </Card>
  )
}
