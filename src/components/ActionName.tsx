import { Group, Text, Tooltip } from '@mantine/core'

// ActionName shows an action by its label, the words a person reads; an action that has words keeps
// its code on hover, the name the API, the filters and the exports know it by. An action without
// words (its label is the code itself) is shown as the code.
export function ActionName({ action, label }: { action: string; label: string }) {
  if (label === action) {
    return (
      <Text span inherit>
        {action}
      </Text>
    )
  }
  return (
    <Tooltip label={action}>
      <Text span inherit>
        {label}
      </Text>
    </Tooltip>
  )
}

// ActionOption is a dropdown option of a field whose value is the action's code: the words, then the
// code it puts in the field.
export function ActionOption({ action, label }: { action: string; label: string }) {
  if (label === action) {
    return <Text size="sm">{action}</Text>
  }
  return (
    <Group gap="xs" wrap="nowrap">
      <Text size="sm">{label}</Text>
      <Text size="xs" c="dimmed">
        {action}
      </Text>
    </Group>
  )
}
