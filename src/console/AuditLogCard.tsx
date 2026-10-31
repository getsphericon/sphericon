import { Button, Card, EmptyState, Stack, Table, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../components/Skeletons.tsx'
import { staffWorkspaceAuditListOptions } from '../generated/staffee/@tanstack/react-query.gen.ts'
import type { StaffAuditEntryResource } from '../generated/staffee/types.gen.ts'
import { formatDateTime } from '../utils/datetime.ts'

const PAGE_SIZE = 25

function ActorLabel({ entry }: { entry: StaffAuditEntryResource }) {
  const { t } = useTranslation()
  const { kind, name, id } = entry.actor
  if (kind === 'system') {
    return t(($) => $.console.workspace.auditLog.actorSystem)
  }
  if (kind === 'api_token') {
    return name ?? t(($) => $.console.workspace.auditLog.actorToken, { id: id ?? '' })
  }
  // A Staff member arrives already as "Sphericon staff", with no id.
  return name ?? id ?? ''
}

function targetLabel({ target }: StaffAuditEntryResource) {
  const detail = target.name ?? (target.id ? `#${target.id}` : '')
  return detail ? `${target.type} ${detail}` : target.type
}

interface AuditPageProps {
  workspaceId: string
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
      errorTitle={t(($) => $.console.workspace.auditLog.loadError)}
    >
      <AuditRows {...props} />
    </QueryBoundary>
  )
}

// The rows of one page; while more exist, a button mounts the next page beneath them, so the
// list grows without holding every page in one state.
function AuditRows({ workspaceId, cursor, first }: AuditPageProps) {
  const { t } = useTranslation()
  const [showNext, setShowNext] = useState(false)
  const { items, nextCursor } = useSuspenseQuery(
    staffWorkspaceAuditListOptions({
      path: { workspaceId },
      query: { limit: PAGE_SIZE, ...(cursor ? { cursor } : {}) },
    }),
  ).data

  const next = nextCursor ?? undefined
  if (first && items.length === 0) {
    return <EmptyState title={t(($) => $.console.workspace.auditLog.empty)} />
  }

  return (
    <>
      <Table.ScrollContainer minWidth={560}>
        <Table withTableBorder>
          {first ? (
            <Table.Thead>
              <Table.Tr>
                <Table.Th>{t(($) => $.console.workspace.auditLog.time)}</Table.Th>
                <Table.Th>{t(($) => $.console.workspace.auditLog.actor)}</Table.Th>
                <Table.Th>{t(($) => $.console.workspace.auditLog.action)}</Table.Th>
                <Table.Th>{t(($) => $.console.workspace.auditLog.target)}</Table.Th>
              </Table.Tr>
            </Table.Thead>
          ) : null}
          <Table.Tbody>
            {items.map((entry) => (
              <Table.Tr key={entry.id}>
                <Table.Td>{formatDateTime(entry.occurredAt)}</Table.Td>
                <Table.Td>
                  <ActorLabel entry={entry} />
                </Table.Td>
                <Table.Td>{entry.action}</Table.Td>
                <Table.Td>{targetLabel(entry)}</Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
      {next && !showNext ? (
        <Button variant="light" onClick={() => setShowNext(true)}>
          {t(($) => $.console.workspace.auditLog.loadMore)}
        </Button>
      ) : null}
      {next && showNext ? (
        <AuditPage workspaceId={workspaceId} cursor={next} first={false} />
      ) : null}
    </>
  )
}

// A Workspace's Audit log, newest first: who changed what, so a Staff member can see recent
// changes made by its team.
export function AuditLogCard({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()

  return (
    <Card withBorder>
      <Stack>
        <Title order={4}>{t(($) => $.console.workspace.auditLog.title)}</Title>
        <AuditPage workspaceId={workspaceId} first />
      </Stack>
    </Card>
  )
}
