import { Code, Group, Stack, Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { SiteEventResource } from '../generated/site/types.gen.ts'
import { useDateFormat } from '../hooks/useDateFormat.ts'

// Expanded-row body for tracking-event tables. `detailed` adds the heading and the
// created-at row used by the workspace activity feed.
export function EventRowDetails({
  record,
  detailed = false,
}: {
  record: SiteEventResource
  detailed?: boolean
}) {
  const { formatDateTime } = useDateFormat()
  const { t } = useTranslation()
  return (
    <Stack p="md" gap="xs">
      {detailed ? <Text fw={700}>{t(($) => $.activity.details)}</Text> : null}
      <Group gap="xs">
        <Text fw={600}>{t(($) => $.activity.subjectId)}:</Text>
        <Text>{record.subjectId}</Text>
      </Group>
      <Group gap="xs">
        <Text fw={600}>{t(($) => $.activity.occurredAt)}:</Text>
        <Text>{record.occurredAt ? formatDateTime(record.occurredAt) : '—'}</Text>
      </Group>
      {detailed ? (
        <Group gap="xs">
          <Text fw={600}>{t(($) => $.activity.createdAt)}:</Text>
          <Text>{formatDateTime(record.createdAt)}</Text>
        </Group>
      ) : null}
      <Stack gap="xs">
        <Text fw={600}>{t(($) => $.activity.properties)}:</Text>
        <Code block>{record.properties ? JSON.stringify(record.properties, null, 2) : '{}'}</Code>
      </Stack>
    </Stack>
  )
}

export function DetailedEventRowDetails({ record }: { record: SiteEventResource }) {
  return <EventRowDetails record={record} detailed />
}
