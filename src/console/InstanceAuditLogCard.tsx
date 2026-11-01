import { Button, Card, EmptyState, Stack, Table, Text, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AuditActionName } from '../components/AuditActionName.tsx'
import { QueryBoundary } from '../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../components/Skeletons.tsx'
import { staffInstanceListAuditEntriesOptions } from '../generated/staff/@tanstack/react-query.gen.ts'
import type { StaffInstanceAuditEntryResource } from '../generated/staff/types.gen.ts'
import { formatDateTime } from '../utils/datetime.ts'

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

interface AuditPageProps {
  cursor?: string
  first: boolean
}

// One page of the log behind its own boundary, so the next page loads, or fails, beneath the
// rows already shown.
function AuditPage(props: AuditPageProps) {
  const { t } = useTranslation()
  return (
    <QueryBoundary
      fallback={<BlockSkeleton />}
      errorTitle={t(($) => $.console.instance.auditLog.loadError)}
    >
      <AuditRows {...props} />
    </QueryBoundary>
  )
}

// The rows of one page; while more exist, a button mounts the next page beneath them.
function AuditRows({ cursor, first }: AuditPageProps) {
  const { t } = useTranslation()
  const [showNext, setShowNext] = useState(false)
  const { items, nextCursor } = useSuspenseQuery(
    staffInstanceListAuditEntriesOptions({
      query: { limit: PAGE_SIZE, ...(cursor ? { cursor } : {}) },
    }),
  ).data

  const next = nextCursor ?? undefined
  if (first && items.length === 0) {
    return <EmptyState title={t(($) => $.console.instance.auditLog.empty)} />
  }

  return (
    <>
      <Table.ScrollContainer minWidth={560}>
        <Table withTableBorder>
          {first ? (
            <Table.Thead>
              <Table.Tr>
                <Table.Th>{t(($) => $.console.instance.auditLog.time)}</Table.Th>
                <Table.Th>{t(($) => $.console.instance.auditLog.actor)}</Table.Th>
                <Table.Th>{t(($) => $.console.instance.auditLog.action)}</Table.Th>
                <Table.Th>{t(($) => $.console.instance.auditLog.changes)}</Table.Th>
              </Table.Tr>
            </Table.Thead>
          ) : null}
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
      {next && !showNext ? (
        <Button variant="light" onClick={() => setShowNext(true)}>
          {t(($) => $.console.instance.auditLog.loadMore)}
        </Button>
      ) : null}
      {next && showNext ? <AuditPage cursor={next} first={false} /> : null}
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
        <AuditPage first />
      </Stack>
    </Card>
  )
}
