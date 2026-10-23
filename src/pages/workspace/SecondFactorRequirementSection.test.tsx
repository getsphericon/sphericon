import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteWorkspacesList,
  handleSiteWorkspacesSetSecondFactorRequirement,
} from '../../generated/site/msw.gen.ts'
import type { SiteWorkspaceResource } from '../../generated/site/types.gen.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { formatDate } from '../../utils/datetime.ts'
import { SecondFactorRequirementSection } from './SecondFactorRequirementSection.tsx'

const NOW = '2026-01-01T00:00:00Z'

const workspace = (over: Partial<SiteWorkspaceResource> = {}): SiteWorkspaceResource => ({
  id: 'AcmeWs01',
  name: 'Acme',

  collectKey: 'ck',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  mcpUrl: 'https://sphericon.test',
  holds: [],
  role: 'member',
  createdAt: NOW,
  ...over,
})

const listRoute = handleSiteWorkspacesList({ body: [workspace()] })

test('an owner switches the requirement on', async () => {
  const bodies: string[] = []
  worker.use(
    listRoute,
    handleSiteWorkspacesSetSecondFactorRequirement(async ({ request }) => {
      bodies.push(await request.text())
      return HttpResponse.json(workspace({ secondFactorRequiredAt: '2026-03-01T00:00:00Z' }))
    }),
  )
  const { screen } = await renderWithRouter(
    <SecondFactorRequirementSection workspace={workspace({ role: 'owner' })} />,
  )

  const toggle = screen.getByRole('switch', { name: 'Require two-factor authentication' })
  await expect.element(toggle).toBeEnabled()
  await toggle.click()

  await expect.poll(() => bodies).toEqual(['{"required":true}'])
})

test('a member sees the requirement and its grace but cannot change it', async () => {
  worker.use(listRoute)
  const required = workspace({ secondFactorRequiredAt: '2026-03-01T00:00:00Z' })
  const { screen } = await renderWithRouter(<SecondFactorRequirementSection workspace={required} />)

  const toggle = screen.getByRole('switch', { name: 'Require two-factor authentication' })
  await expect.element(toggle).toBeChecked()
  await expect.element(toggle).toBeDisabled()
  await expect
    .element(screen.getByText(new RegExp(`have until ${formatDate('2026-03-08T00:00:00Z')}`)))
    .toBeInTheDocument()
})
