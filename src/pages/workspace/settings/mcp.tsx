import {
  ActionIcon,
  Alert,
  Anchor,
  Box,
  Button,
  CopyButton,
  EmptyState,
  Stack,
  Text,
  TextInput,
  Title,
  Tooltip,
} from '@mantine/core'
import { IconCheck, IconCopy } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useTranslation } from 'react-i18next'

import { ScopesBadge } from '../../../components/ScopesBadge.tsx'
import { TableSkeleton } from '../../../components/Skeletons.tsx'
import {
  siteConnectionsDeleteMutation,
  siteConnectionsListOptions,
  siteConnectionsListQueryKey,
} from '../../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { useConfirmation } from '../../../hooks/useConfirmation.tsx'
import { useResourceMutation } from '../../../hooks/useResourceMutation.ts'
import { Route as mcpRoute } from '../../../routes/workspaces/$workspaceId/settings/mcp.tsx'
import { formatDate } from '../../../utils/datetime.ts'
import { DOCS_URL } from '../../../utils/docs.ts'
import { can } from '../../../utils/permissions.ts'
import { SettingsFrame } from './SettingsFrame.tsx'

// The guide with the Bearer token setup the app does not repeat.
const MCP_GUIDE_URL = `${DOCS_URL}/guide/mcp`

// McpSection tells how to connect an AI agent: the endpoint to paste into a connector
// and what approving it does. Any member may approve one (ADR 0061).
function McpSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const url = `${workspace.mcpUrl.replace(/\/$/, '')}/mcp`

  return (
    <Box>
      <Text c="dimmed" size="sm" mb="sm">
        {t(($) => $.settings.mcp.description)}
      </Text>
      <TextInput
        readOnly
        maw={480}
        value={url}
        aria-label={t(($) => $.settings.mcp.addressLabel)}
        rightSection={
          <CopyButton value={url}>
            {({ copied, copy }) => (
              <Tooltip label={copied ? t(($) => $.settings.copied) : t(($) => $.settings.copy)}>
                <ActionIcon
                  variant="subtle"
                  color={copied ? 'teal' : 'gray'}
                  onClick={copy}
                  aria-label={t(($) => $.settings.copy)}
                >
                  {copied ? <IconCheck size={16} /> : <IconCopy size={16} />}
                </ActionIcon>
              </Tooltip>
            )}
          </CopyButton>
        }
      />
      <Text size="sm" mt="md">
        {t(($) => $.settings.mcp.approveNote)}
      </Text>
      <Anchor href={MCP_GUIDE_URL} target="_blank" rel="noreferrer" size="sm" mt="xs">
        {t(($) => $.settings.mcp.guideLink)}
      </Anchor>
    </Box>
  )
}

// ConnectionsSection lists the Connections the caller may see (their own, or everyone's with
// members:manage, which the server decides) and revokes one. They are not API tokens and are not
// listed with them.
function ConnectionsSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const confirmRevoke = useConfirmation()
  const workspaceId = workspace.id
  const queryKey = siteConnectionsListQueryKey({ path: { workspaceId } })
  const connections = useQuery(siteConnectionsListOptions({ path: { workspaceId } }))
  const revoke = useResourceMutation({
    mutation: siteConnectionsDeleteMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.connections.revokeError),
  })
  const onRevoke = (id: string) => {
    confirmRevoke({
      title: t(($) => $.settings.connections.revokeConfirmTitle),
      description: t(($) => $.settings.connections.revokeConfirmDescription),
      confirmLabel: t(($) => $.settings.connections.revoke),
      onConfirm: () => revoke.mutate({ path: { workspaceId, id } }),
    })
  }
  const records = connections.data ?? []

  return (
    <Box>
      <Title order={4} mb="xs">
        {t(($) => $.settings.connections.title)}
      </Title>
      <Text c="dimmed" size="sm" mb="sm">
        {can(workspace, 'members:manage')
          ? t(($) => $.settings.connections.descriptionManager)
          : t(($) => $.settings.connections.description)}
      </Text>
      {connections.isError ? (
        <Alert color="red" title={t(($) => $.settings.connections.loadError)} mb="md" />
      ) : null}
      {connections.isLoading ? <TableSkeleton /> : null}
      <DataTable
        minHeight={records.length === 0 ? 160 : 0}
        withTableBorder
        records={records}
        idAccessor="id"
        columns={[
          { accessor: 'name', title: t(($) => $.settings.connections.application) },
          {
            accessor: 'userName',
            title: t(($) => $.settings.connections.user),
            render: (record) => `${record.userName} (${record.userEmail})`,
          },
          {
            accessor: 'scopes',
            title: t(($) => $.settings.connections.scopes),
            render: (record) => <ScopesBadge scopes={record.scopes} />,
          },
          {
            accessor: 'createdAt',
            title: t(($) => $.settings.connections.created),
            render: (record) => formatDate(record.createdAt),
          },
          {
            accessor: 'actions',
            title: '',
            textAlign: 'right',
            render: (record) => (
              <Button
                size="xs"
                variant="subtle"
                color="red"
                onClick={() => onRevoke(record.id)}
                aria-label={t(($) => $.settings.connections.revokeLabel, { name: record.name })}
              >
                {t(($) => $.settings.connections.revoke)}
              </Button>
            ),
          },
        ]}
        emptyState={<EmptyState title={t(($) => $.settings.connections.empty)} />}
      />
    </Box>
  )
}

export function McpPage() {
  const { t } = useTranslation()
  const { workspaceId } = mcpRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.mcp)}>
      {(workspace) => (
        <Stack>
          <McpSection workspace={workspace} />
          <ConnectionsSection workspace={workspace} />
        </Stack>
      )}
    </SettingsFrame>
  )
}
