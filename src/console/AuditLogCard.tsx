import { Card, EmptyState, Stack, Table, Title } from '@mantine/core'
import { useSuspenseInfiniteQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { AuditActionName } from '../components/AuditActionName.tsx'
import { LoadMore } from '../components/LoadMore.tsx'
import { QueryBoundary } from '../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../components/Skeletons.tsx'
import {
  staffWorkspaceAuditListInfiniteOptions,
  staffWorkspaceAuditListQueryKey,
} from '../generated/staffee/@tanstack/react-query.gen.ts'
import type { StaffAuditEntryResource } from '../generated/staffee/types.gen.ts'
import { suspenseInfinite } from '../utils/suspenseInfinite.ts'
import { formatDateTime } from './format.ts'

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

// The log's entries, read a page at a time: the first under the card's boundary, each next one
// beneath the rows already shown.
function AuditRows({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const options = { path: { workspaceId }, query: { limit: PAGE_SIZE } }
  const query = useSuspenseInfiniteQuery({
    ...suspenseInfinite(
      staffWorkspaceAuditListInfiniteOptions(options),
      staffWorkspaceAuditListQueryKey(options),
    ),
    // The empty cursor is the log's start.
    initialPageParam: '',
    getNextPageParam: (page) => page.nextCursor ?? undefined,
  })
  const items = query.data.pages.flatMap((page) => page.items)

  if (items.length === 0) {
    return <EmptyState title={t(($) => $.console.workspace.auditLog.empty)} />
  }

  return (
    <>
      <Table.ScrollContainer minWidth={560}>
        <Table withTableBorder>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>{t(($) => $.console.workspace.auditLog.time)}</Table.Th>
              <Table.Th>{t(($) => $.console.workspace.auditLog.actor)}</Table.Th>
              <Table.Th>{t(($) => $.console.workspace.auditLog.action)}</Table.Th>
              <Table.Th>{t(($) => $.console.workspace.auditLog.target)}</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {items.map((entry) => (
              <Table.Tr key={entry.id}>
                <Table.Td>{formatDateTime(entry.occurredAt)}</Table.Td>
                <Table.Td>
                  <ActorLabel entry={entry} />
                </Table.Td>
                <Table.Td>
                  <AuditActionName action={entry.action} />
                </Table.Td>
                <Table.Td>{targetLabel(entry)}</Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
      <LoadMore
        query={query}
        label={t(($) => $.console.workspace.auditLog.loadMore)}
        errorTitle={t(($) => $.console.workspace.auditLog.loadError)}
      />
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
        <QueryBoundary
          fallback={<BlockSkeleton />}
          errorTitle={t(($) => $.console.workspace.auditLog.loadError)}
        >
          <AuditRows workspaceId={workspaceId} />
        </QueryBoundary>
      </Stack>
    </Card>
  )
}
