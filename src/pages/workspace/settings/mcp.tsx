import { Box, ActionIcon, Anchor, CopyButton, Stack, Text, TextInput, Tooltip } from '@mantine/core'
import { IconCheck, IconCopy } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { AnchorLink } from '../../../components/RouterLink.tsx'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { Route as apiKeysRoute } from '../../../routes/workspaces/$workspaceId/settings/api-keys.tsx'
import { Route as mcpRoute } from '../../../routes/workspaces/$workspaceId/settings/mcp.tsx'
import { DOCS_URL } from '../../../utils/docs.ts'
import { can } from '../../../utils/permissions.ts'
import { SettingsFrame } from './SettingsFrame.tsx'

// The guide with the Bearer token setup the app does not repeat.
const MCP_GUIDE_URL = `${DOCS_URL}/guide/mcp`

// McpSection tells how to connect an AI agent: the endpoint to paste into a connector
// and what approving it does. Everyone sees it, but approving needs tokens:manage.
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
        {t(($) => $.settings.mcp.tokenNote)}{' '}
        <AnchorLink to={apiKeysRoute.to} params={{ workspaceId: workspace.id }} size="sm">
          {t(($) => $.settings.mcp.apiKeysLink)}
        </AnchorLink>
      </Text>
      <Text size="sm" mt="xs">
        {t(($) => $.settings.mcp.approveNote)}
        {!can(workspace, 'tokens:manage') ? ` ${t(($) => $.settings.mcp.memberNote)}` : ''}
      </Text>
      <Anchor href={MCP_GUIDE_URL} target="_blank" rel="noreferrer" size="sm" mt="xs">
        {t(($) => $.settings.mcp.guideLink)}
      </Anchor>
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
        </Stack>
      )}
    </SettingsFrame>
  )
}
