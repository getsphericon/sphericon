import { beforeEach, expect, test } from 'vitest'

import { handleSiteWorkspacesList } from '../generated/site/msw.gen.ts'
import { Route as overviewRoute } from '../routes/workspaces/$slug/index.tsx'
import { Route as developersRoute } from '../routes/workspaces/$slug/settings/developers.tsx'
import { Route as generalRoute } from '../routes/workspaces/$slug/settings/index.tsx'
import { Route as mcpRoute } from '../routes/workspaces/$slug/settings/mcp.tsx'
import { Route as privacyRoute } from '../routes/workspaces/$slug/settings/privacy.tsx'
import { Route as sendingRoute } from '../routes/workspaces/$slug/settings/sending.tsx'
import { Route as teamRoute } from '../routes/workspaces/$slug/settings/team.tsx'
import { Route as trackingRoute } from '../routes/workspaces/$slug/settings/tracking.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { worker } from '../test/worker.ts'
import { SettingsNavbar } from './SettingsNavbar.tsx'

const LINKS = [
  ['Back to workspace', overviewRoute],
  ['General', generalRoute],
  ['Team and security', teamRoute],
  ['Tracking', trackingRoute],
  ['Sending', sendingRoute],
  ['Developers', developersRoute],
  ['AI agents (MCP)', mcpRoute],
  ['Privacy requests', privacyRoute],
] as const

const workspace = (role: 'owner' | 'admin' | 'member') => ({
  id: '1',
  name: 'Acme',
  slug: 'acme',
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

test('a plain member is not offered Privacy requests', async () => {
  worker.use(handleSiteWorkspacesList({ body: [workspace('member')] }))

  const { screen } = await renderWithRouter(<SettingsNavbar slug="acme" />)

  await expect
    .element(screen.getByRole('link', { name: 'General', exact: true }))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Privacy requests', exact: true }))
    .not.toBeInTheDocument()
})
