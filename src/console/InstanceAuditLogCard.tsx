import { Card, EmptyState, Stack, Table, Text, Title } from '@mantine/core'
import { useSuspenseInfiniteQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { AuditActionName } from '../components/AuditActionName.tsx'
import { LoadMore } from '../components/LoadMore.tsx'
import { QueryBoundary } from '../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../components/Skeletons.tsx'
import {
  staffInstanceListAuditEntriesInfiniteOptions,
  staffInstanceListAuditEntriesQueryKey,
} from '../generated/staff/@tanstack/react-query.gen.ts'
import type { StaffInstanceAuditEntryResource } from '../generated/staff/types.gen.ts'
import { suspenseInfinite } from '../utils/suspenseInfinite.ts'
import { formatDateTime } from './format.ts'

const PAGE_SIZE = 25

// One side of a changed field, absent where there was no value (no "from") or it was cleared (no
// "to").
function side(change: unknown, key: 'from' | 'to'): string | undefined {
  if (typeof change !== 'object' || change === null || !(key in change)) {
    return undefined
  }
  return String(Reflect.get(change, key))
}

// One changed field of an entry: its old and its new value.
function FieldChange({ field, change }: { field: string; change: unknown }) {
  const { t } = useTranslation()
  const none = t(($) => $.console.instance.auditLog.noValue)
  return (
    <Text size="sm">
      {t(($) => $.console.instance.auditLog.change, {
        field,
        from: side(change, 'from') ?? none,
        to: side(change, 'to') ?? none,
      })}
    </Text>
  )
}

function Changes({ entry }: { entry: StaffInstanceAuditEntryResource }) {
  return Object.entries(entry.diff).map(([field, change]) => (
    <FieldChange key={field} field={field} change={change} />
  ))
}

// The log's entries, read a page at a time: the first under the card's boundary, each next one
// beneath the rows already shown.
function AuditRows() {
  const { t } = useTranslation()
  const options = { query: { limit: PAGE_SIZE } }
  const query = useSuspenseInfiniteQuery({
    ...suspenseInfinite(
      staffInstanceListAuditEntriesInfiniteOptions(options),
      staffInstanceListAuditEntriesQueryKey(options),
    ),
    // The empty cursor is the log's start.
    initialPageParam: '',
    getNextPageParam: (page) => page.nextCursor ?? undefined,
  })
  const items = query.data.pages.flatMap((page) => page.items)

  if (items.length === 0) {
    return <EmptyState title={t(($) => $.console.instance.auditLog.empty)} />
  }

  return (
    <>
      <Table.ScrollContainer minWidth={560}>
        <Table withTableBorder>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>{t(($) => $.console.instance.auditLog.time)}</Table.Th>
              <Table.Th>{t(($) => $.console.instance.auditLog.actor)}</Table.Th>
              <Table.Th>{t(($) => $.console.instance.auditLog.action)}</Table.Th>
              <Table.Th>{t(($) => $.console.instance.auditLog.changes)}</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {items.map((entry) => (
              <Table.Tr key={entry.id}>
                <Table.Td>{formatDateTime(entry.occurredAt)}</Table.Td>
                <Table.Td>{entry.actor.email}</Table.Td>
                <Table.Td>
                  <AuditActionName action={entry.action} />
                </Table.Td>
                <Table.Td>
                  <Changes entry={entry} />
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
      <LoadMore
        query={query}
        label={t(($) => $.console.instance.auditLog.loadMore)}
        errorTitle={t(($) => $.console.instance.auditLog.loadError)}
      />
    </>
  )
}

// The Instance audit log (ADR 0070), newest first: which Staff member changed which setting of the
// Instance itself, from what to what.
export function InstanceAuditLogCard() {
  const { t } = useTranslation()

  return (
    <Card withBorder>
      <Stack>
        <Title order={4}>{t(($) => $.console.instance.auditLog.title)}</Title>
        <Text size="sm" c="dimmed">
          {t(($) => $.console.instance.auditLog.description)}
        </Text>
        <QueryBoundary
          fallback={<BlockSkeleton />}
          errorTitle={t(($) => $.console.instance.auditLog.loadError)}
        >
          <AuditRows />
        </QueryBoundary>
      </Stack>
    </Card>
  )
}
