import { expect, test } from 'vitest'

import { Route as overviewRoute } from '../routes/workspaces/$slug/index.tsx'
import { Route as developersRoute } from '../routes/workspaces/$slug/settings/developers.tsx'
import { Route as generalRoute } from '../routes/workspaces/$slug/settings/index.tsx'
import { Route as mcpRoute } from '../routes/workspaces/$slug/settings/mcp.tsx'
import { Route as sendingRoute } from '../routes/workspaces/$slug/settings/sending.tsx'
import { Route as teamRoute } from '../routes/workspaces/$slug/settings/team.tsx'
import { Route as trackingRoute } from '../routes/workspaces/$slug/settings/tracking.tsx'
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
  const { screen } = await renderWithRouter(<SettingsNavbar slug="acme" />)

  await expect
    .element(screen.getByRole('link', { name: label, exact: true }))
    .toHaveAttribute('href', routeMount(target, { slug: 'acme' }).initialPath)
})

test('General is not current on a page under settings', async () => {
  const { screen } = await renderWithRouter(
    <SettingsNavbar slug="acme" />,
    routeMount(teamRoute, { slug: 'acme' }),
  )

  await expect
    .element(screen.getByRole('link', { name: 'Team and security', exact: true }))
    .toHaveAttribute('aria-current', 'page')
  await expect
    .element(screen.getByRole('link', { name: 'General', exact: true }))
    .not.toHaveAttribute('aria-current')
})
