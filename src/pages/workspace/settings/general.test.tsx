import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteWorkspacesList,
  handleSiteWorkspacesUpdate,
} from '../../../generated/site/msw.gen.ts'
import { Route as generalRoute } from '../../../routes/workspaces/$slug/settings/index.tsx'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { GeneralPage } from './general.tsx'

const GENERAL_ROUTE = routeMount(generalRoute, { slug: 'test' })

const workspace = {
  id: '1',
  name: 'Acme',
  slug: 'test',
  collectKey: 'omck_test_key',
  postalAddress: '',
  accountLinkAvailable: false,
  mcpUrl: 'https://sphericon.test',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

test('renames the workspace', async () => {
  const puts: string[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteWorkspacesUpdate(async ({ request }) => {
      puts.push(await request.text())
      return HttpResponse.json({ ...workspace, name: 'Acme Inc' })
    }),
  )

  const { screen } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  const nameInput = screen.getByLabelText(/^Workspace name/)
  await expect.element(nameInput).toHaveValue('Acme')
  await nameInput.fill('Acme Inc')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => puts.length).toBeGreaterThan(0)
  expect(puts[0]).toContain('"name":"Acme Inc"')
})

test('shows an error alert when the workspaces fail to load', async () => {
  worker.use(handleSiteWorkspacesList(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  await expect.element(screen.getByRole('alert', { name: /Failed to load/ })).toBeInTheDocument()
})
