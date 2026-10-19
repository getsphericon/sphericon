import { Button, Card, Code, CopyButton, List, Stack, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { AnchorLink } from '../../../components/RouterLink.tsx'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { Route as developersRoute } from '../../../routes/workspaces/$slug/settings/developers.tsx'
import { Route as mcpRoute } from '../../../routes/workspaces/$slug/settings/mcp.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

// McpSection tells how to connect an AI agent: the endpoint to paste into a connector
// and what approving it does. Everyone sees it, but only an Owner or Admin can approve.
function McpSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const url = `${workspace.mcpUrl.replace(/\/$/, '')}/mcp`

  return (
    <Card withBorder>
      <Title order={4} mb="xs">
        {t(($) => $.settings.mcp.title)}
      </Title>
      <Text c="dimmed" size="sm" mb="sm">
        {t(($) => $.settings.mcp.description)}
      </Text>
      <Code block>{url}</Code>
      <CopyButton value={url}>
        {({ copied, copy }) => (
          <Button mt="sm" variant="light" onClick={copy}>
            {copied ? t(($) => $.settings.copied) : t(($) => $.settings.copy)}
          </Button>
        )}
      </CopyButton>
      <List type="ordered" size="sm" mt="md" spacing="xs">
        <List.Item>{t(($) => $.settings.mcp.stepAdd)}</List.Item>
        <List.Item>{t(($) => $.settings.mcp.stepConnect)}</List.Item>
        <List.Item>{t(($) => $.settings.mcp.stepApprove)}</List.Item>
      </List>
      <Text size="sm" mt="md">
        {t(($) => $.settings.mcp.tokenNote)}{' '}
        <AnchorLink to={developersRoute.to} params={{ slug: workspace.slug }} size="sm">
          {t(($) => $.settings.mcp.apiKeysLink)}
        </AnchorLink>
      </Text>
      <Text size="sm" mt="xs">
        {t(($) => $.settings.mcp.approveNote)}
        {workspace.role === 'member' ? ` ${t(($) => $.settings.mcp.memberNote)}` : ''}
      </Text>
    </Card>
  )
}

export function McpPage() {
  const { t } = useTranslation()
  const { slug } = mcpRoute.useParams()

  return (
    <SettingsFrame slug={slug} title={t(($) => $.settings.nav.mcp)}>
      {(workspace) => (
        <Stack>
          <McpSection workspace={workspace} />
        </Stack>
      )}
    </SettingsFrame>
  )
}
