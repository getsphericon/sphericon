import { Button, Card, Loader, Stack, Table, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../components/ApiErrorAlert.tsx'
import { operatorWorkspaceAuditListOptions } from '../generated/operator/@tanstack/react-query.gen.ts'
import type { OperatorAuditEntryResource } from '../generated/operator/types.gen.ts'
import { formatDateTime } from '../utils/datetime.ts'

const PAGE_SIZE = 25

function ActorLabel({ entry }: { entry: OperatorAuditEntryResource }) {
  const { t } = useTranslation()
  const { kind, name, id } = entry.actor
  if (kind === 'system') return <>{t(($) => $.console.workspace.auditLog.actorSystem)}</>
  if (kind === 'api_token') {
    return <>{name ?? t(($) => $.console.workspace.auditLog.actorToken, { id: id ?? '' })}</>
  }
  // An Operator arrives already as "sphericon staff", with no id.
  return <>{name ?? id ?? ''}</>
}

function targetLabel({ target }: OperatorAuditEntryResource) {
  const detail = target.name ?? (target.id ? `#${target.id}` : '')
  return detail ? `${target.type} ${detail}` : target.type
}

// One page of the log; while more exist, a button mounts the next page beneath it, so
// the list grows without holding every page in one state.
function AuditPage({
  workspaceId,
  cursor,
  first,
}: {
  workspaceId: string
  cursor?: string
  first: boolean
}) {
  const { t } = useTranslation()
  const [showNext, setShowNext] = useState(false)
  const query = useQuery(
    operatorWorkspaceAuditListOptions({
      path: { workspaceId },
      query: { limit: PAGE_SIZE, ...(cursor ? { cursor } : {}) },
    }),
  )

  if (query.isLoading) return <Loader size="sm" />
  if (query.isError) {
    return (
      <ApiErrorAlert
        error={query.error}
        title={t(($) => $.console.workspace.auditLog.loadError)}
        fallback={t(($) => $.console.workspace.auditLog.loadError)}
      />
    )
  }

  const items = query.data?.items ?? []
  const next = query.data?.nextCursor ?? undefined
  if (first && items.length === 0) {
    return (
      <Text c="dimmed" size="sm">
        {t(($) => $.console.workspace.auditLog.empty)}
      </Text>
    )
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

// A Workspace's Audit log, newest first: who changed what, so an Operator can see recent
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
