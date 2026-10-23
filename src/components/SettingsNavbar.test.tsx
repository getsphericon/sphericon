import { beforeEach, expect, test } from 'vitest'

import { handleSiteWorkspacesList } from '../generated/site/msw.gen.ts'
import type { SiteMembershipRole } from '../generated/site/types.gen.ts'
import { Route as overviewRoute } from '../routes/workspaces/$workspaceId/index.tsx'
import { Route as apiKeysRoute } from '../routes/workspaces/$workspaceId/settings/api-keys.tsx'
import { Route as auditLogRoute } from '../routes/workspaces/$workspaceId/settings/audit-log.tsx'
import { Route as emailsRoute } from '../routes/workspaces/$workspaceId/settings/emails.tsx'
import { Route as generalRoute } from '../routes/workspaces/$workspaceId/settings/index.tsx'
import { Route as mcpRoute } from '../routes/workspaces/$workspaceId/settings/mcp.tsx'
import { Route as privacyRoute } from '../routes/workspaces/$workspaceId/settings/privacy.tsx'
import { Route as teamRoute } from '../routes/workspaces/$workspaceId/settings/team.tsx'
import { Route as trackingRoute } from '../routes/workspaces/$workspaceId/settings/tracking.tsx'
import { Route as webhooksRoute } from '../routes/workspaces/$workspaceId/settings/webhooks.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { worker } from '../test/worker.ts'
import { SettingsNavbar } from './SettingsNavbar.tsx'

const LINKS = [
  ['Back to workspace', overviewRoute],
  ['General', generalRoute],
  ['Team and security', teamRoute],
  ['Tracking', trackingRoute],
  ['Emails', emailsRoute],
  ['Audit log', auditLogRoute],
  ['API keys', apiKeysRoute],
  ['Webhooks', webhooksRoute],
  ['AI agents (MCP)', mcpRoute],
  ['Privacy requests', privacyRoute],
] as const

const workspace = (role: SiteMembershipRole) => ({
  id: 'AcmeWs01',
  name: 'Acme',
  collectKey: 'omck_test_key',
  postalAddress: '',
  accountLinkAvailable: false,
  mcpUrl: 'https://mcp.sphericon.test/',
  holds: [],
  role,
  createdAt: '2026-01-01T00:00:00Z',
})

beforeEach(() => {
  worker.use(handleSiteWorkspacesList({ body: [workspace('owner')] }))
})

test.each(LINKS)('%s links to its workspace-scoped route', async (label, target) => {
  const { screen } = await renderWithRouter(<SettingsNavbar workspaceId="AcmeWs01" />)

  await expect
    .element(screen.getByRole('link', { name: label, exact: true }))
    .toHaveAttribute('href', routeMount(target, { workspaceId: 'AcmeWs01' }).initialPath)
})

test('General is not current on a page under settings', async () => {
  const { screen } = await renderWithRouter(
    <SettingsNavbar workspaceId="AcmeWs01" />,
    routeMount(teamRoute, { workspaceId: 'AcmeWs01' }),
  )

  await expect
    .element(screen.getByRole('link', { name: 'Team and security', exact: true }))
    .toHaveAttribute('aria-current', 'page')
  await expect
    .element(screen.getByRole('link', { name: 'General', exact: true }))
    .not.toHaveAttribute('aria-current')
})

test('a plain member is not offered Webhooks or Privacy requests', async () => {
  worker.use(handleSiteWorkspacesList({ body: [workspace('member')] }))

  const { screen } = await renderWithRouter(<SettingsNavbar workspaceId="AcmeWs01" />)

  await expect
    .element(screen.getByRole('link', { name: 'General', exact: true }))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Webhooks', exact: true }))
    .not.toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Privacy requests', exact: true }))
    .not.toBeInTheDocument()
})
