import { expect, test } from 'vitest'

import { Route as overviewRoute } from '../routes/workspaces/$workspaceId/index.tsx'
import { Route as developersRoute } from '../routes/workspaces/$workspaceId/settings/developers.tsx'
import { Route as generalRoute } from '../routes/workspaces/$workspaceId/settings/index.tsx'
import { Route as mcpRoute } from '../routes/workspaces/$workspaceId/settings/mcp.tsx'
import { Route as sendingRoute } from '../routes/workspaces/$workspaceId/settings/sending.tsx'
import { Route as teamRoute } from '../routes/workspaces/$workspaceId/settings/team.tsx'
import { Route as trackingRoute } from '../routes/workspaces/$workspaceId/settings/tracking.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { SettingsNavbar } from './SettingsNavbar.tsx'

const LINKS = [
  ['Back to workspace', overviewRoute],
  ['General', generalRoute],
  ['Team and security', teamRoute],
  ['Tracking', trackingRoute],
  ['Sending', sendingRoute],
  ['Developers', developersRoute],
  ['AI agents (MCP)', mcpRoute],
] as const

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
