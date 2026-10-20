import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteWorkspacesDelete,
  handleSiteWorkspacesList,
  handleSiteWorkspacesUpdate,
} from '../../../generated/site/msw.gen.ts'
import { Route as generalRoute } from '../../../routes/workspaces/$workspaceId/settings/index.tsx'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { GeneralPage } from './general.tsx'

const GENERAL_ROUTE = routeMount(generalRoute, { workspaceId: 'TestWs01' })

const workspace = {
  id: 'TestWs01',
  name: 'Acme',

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
  // The General section's Save comes first; the Brand kit has its own.
  await screen.getByRole('button', { name: 'Save' }).first().click()

  await expect.poll(() => puts.length).toBeGreaterThan(0)
  expect(puts[0]).toContain('"name":"Acme Inc"')
})

test('a plain member sees the name read-only', async () => {
  worker.use(handleSiteWorkspacesList({ body: [{ ...workspace, role: 'member' as const }] }))

  const { screen } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  await expect.element(screen.getByLabelText(/^Workspace name/)).toBeDisabled()
})

test('shows an error alert when the workspaces fail to load', async () => {
  worker.use(handleSiteWorkspacesList(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  await expect.element(screen.getByRole('alert', { name: /Failed to load/ })).toBeInTheDocument()
})

test('an owner deletes the workspace only after typing its name', async () => {
  const posts: string[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteWorkspacesDelete(async ({ request }) => {
      posts.push(await request.text())
      return HttpResponse.json({
        id: workspace.id,
        name: workspace.name,
        discardedAt: '2026-02-01T00:00:00Z',
        eraseAt: '2026-03-03T00:00:00Z',
        onlyOwner: false,
      })
    }),
  )

  const { screen, navigate } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  await screen.getByRole('button', { name: 'Delete workspace' }).click()
  const confirm = screen.getByRole('dialog').getByRole('button', { name: 'Delete workspace' })
  await expect.element(confirm).toBeDisabled()

  await screen.getByLabelText(/^Type Acme to confirm/).fill('Acme')
  await confirm.click()

  await expect.poll(() => posts.length).toBe(1)
  expect(posts[0]).toContain('"name":"Acme"')
  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: '/' }])
})

test('an admin sees no danger section', async () => {
  worker.use(handleSiteWorkspacesList({ body: [{ ...workspace, role: 'admin' as const }] }))

  const { screen } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  await expect.element(screen.getByLabelText(/^Workspace name/)).toBeEnabled()
  expect(screen.container.textContent).not.toContain('Danger zone')
})
