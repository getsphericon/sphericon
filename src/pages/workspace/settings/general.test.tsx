import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteWorkspacesDelete,
  handleSiteWorkspacesLeave,
  handleSiteWorkspacesList,
  handleSiteWorkspacesSetTimezone,
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
  timezone: 'UTC',
  accountLinkAvailable: false,
  mcpUrl: 'https://sphericon.test',
  trackerUrl: 'https://t.sphericon.test',
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

test('an owner sets the time zone', async () => {
  const bodies: string[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteWorkspacesSetTimezone(async ({ request }) => {
      bodies.push(await request.text())
      return HttpResponse.json({ ...workspace, timezone: 'Europe/Berlin' })
    }),
  )

  const { screen } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  const zone = screen.getByRole('combobox', { name: 'Time zone' })
  await expect.element(zone).toHaveValue('UTC')
  await zone.fill('Europe/Berlin')
  await screen.getByRole('option', { name: 'Europe/Berlin' }).click()
  await screen.getByRole('button', { name: 'Save' }).nth(1).click()

  await expect.poll(() => bodies.length).toBe(1)
  expect(bodies[0]).toContain('"timezone":"Europe/Berlin"')
})

test('an admin sees the time zone read-only', async () => {
  worker.use(handleSiteWorkspacesList({ body: [{ ...workspace, role: 'admin' as const }] }))

  const { screen } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  await expect.element(screen.getByRole('combobox', { name: 'Time zone' })).toBeDisabled()
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

test('an admin can leave but not delete the workspace', async () => {
  worker.use(handleSiteWorkspacesList({ body: [{ ...workspace, role: 'admin' as const }] }))

  const { screen } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  await expect.element(screen.getByRole('button', { name: 'Leave workspace' })).toBeInTheDocument()
  await expect
    .element(screen.getByRole('button', { name: 'Delete workspace' }))
    .not.toBeInTheDocument()
})

test('leaves the workspace after confirmation', async () => {
  let left = false
  worker.use(
    handleSiteWorkspacesList({ body: [{ ...workspace, role: 'member' as const }] }),
    handleSiteWorkspacesLeave(() => {
      left = true
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  await screen.getByRole('button', { name: 'Leave workspace' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Leave' }).click()

  await expect.poll(() => left).toBe(true)
  // The index route picks the next Workspace, or the create screen when none is left.
  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: '/' }])
})

test('keeps the workspace and says why when the only owner tries to leave', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteWorkspacesLeave(() =>
      HttpResponse.json(
        {
          title: 'cannot leave as the only owner',
          detail:
            'you are the only owner: make another member an owner or delete the workspace first',
          status: 422,
        },
        { status: 422, headers: { 'content-type': 'application/problem+json' } },
      ),
    ),
  )
  const { screen } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  await screen.getByRole('button', { name: 'Leave workspace' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Leave' }).click()

  await expect.element(screen.getByText(/make another member an owner/)).toBeInTheDocument()
})
