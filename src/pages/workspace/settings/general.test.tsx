import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteWorkspacesDelete,
  handleSiteWorkspacesGet,
  handleSiteWorkspacesGetRevenueSettings,
  handleSiteWorkspacesLeave,
  handleSiteWorkspacesList,
  handleSiteWorkspacesSetTimezone,
  handleSiteWorkspacesUpdate,
} from '../../../generated/site/msw.gen.ts'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { Route as generalRoute } from '../../../routes/workspaces/$workspaceId/settings/index.tsx'
import { problem } from '../../../test/problem.ts'
import { readAgain } from '../../../test/queryClients.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { GeneralPage } from './general.tsx'

const GENERAL_ROUTE = routeMount(generalRoute, { workspaceId: 'TestWs01' })

const revenueSettings = handleSiteWorkspacesGetRevenueSettings({
  body: { reportingCurrency: null, attributionWindowDays: 5, revision: 1 },
})

const workspace: SiteWorkspaceResource = {
  id: 'TestWs01',
  name: 'Acme',

  collectKey: 'omck_test_key',
  revision: 1,
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions: ['workspace:update', 'workspace:timezone', 'workspace:delete'],
  mcpUrl: 'https://sphericon.test',
  trackerUrl: 'https://t.sphericon.test',
  shortLinkLength: 35,
  smsMaxLength: { gsm7: 1600, ucs2: 700 },
  holds: [],
  role: 'owner',
  createdAt: '2026-01-01T00:00:00Z',
}

test('renames the workspace', async () => {
  const puts: string[] = []
  worker.use(
    revenueSettings,
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteWorkspacesGet({ body: workspace }),
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

test('without workspace:update the name is read-only', async () => {
  worker.use(
    revenueSettings,
    handleSiteWorkspacesList({ body: [{ ...workspace, role: 'member', permissions: [] }] }),
    handleSiteWorkspacesGet({ body: { ...workspace, role: 'member', permissions: [] } }),
  )

  const { screen } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  await expect.element(screen.getByLabelText(/^Workspace name/)).toBeDisabled()
})

test('workspace:timezone sets the time zone', async () => {
  const bodies: string[] = []
  worker.use(
    revenueSettings,
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteWorkspacesGet({ body: workspace }),
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

test('without workspace:timezone the time zone is read-only', async () => {
  worker.use(
    revenueSettings,
    handleSiteWorkspacesList({
      body: [{ ...workspace, role: 'member', permissions: ['workspace:update'] }],
    }),
    handleSiteWorkspacesGet({
      body: { ...workspace, role: 'member', permissions: ['workspace:update'] },
    }),
  )

  const { screen } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  await expect.element(screen.getByRole('combobox', { name: 'Time zone' })).toBeDisabled()
})

test('shows an error alert when the workspaces fail to load', async () => {
  worker.use(
    revenueSettings,
    handleSiteWorkspacesList(() => problem(500, { detail: 'boom' })),
  )
  const { screen } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  await expect.element(screen.getByRole('alert', { name: /Failed to load/ })).toBeInTheDocument()
})

test('workspace:delete deletes the workspace only after typing its name', async () => {
  const posts: string[] = []
  worker.use(
    revenueSettings,
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteWorkspacesGet({ body: workspace }),
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

test('without workspace:delete the workspace can be left but not deleted', async () => {
  worker.use(
    revenueSettings,
    handleSiteWorkspacesList({
      body: [{ ...workspace, role: 'admin', permissions: ['workspace:update'] }],
    }),
    handleSiteWorkspacesGet({
      body: { ...workspace, role: 'admin', permissions: ['workspace:update'] },
    }),
  )

  const { screen } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  await expect.element(screen.getByRole('button', { name: 'Leave workspace' })).toBeInTheDocument()
  await expect
    .element(screen.getByRole('button', { name: 'Delete workspace' }))
    .not.toBeInTheDocument()
})

test('leaves the workspace after confirmation', async () => {
  let left = false
  worker.use(
    revenueSettings,
    handleSiteWorkspacesList({ body: [{ ...workspace, role: 'member', permissions: [] }] }),
    handleSiteWorkspacesGet({ body: { ...workspace, role: 'member', permissions: [] } }),
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
    revenueSettings,
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteWorkspacesGet({ body: workspace }),
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

// The revision a request's body names.
async function revisionOf(request: Request) {
  const body: unknown = await request.json()
  return typeof body === 'object' && body !== null && 'revision' in body ? body.revision : null
}

// The General and Time zone sections are two editors of one Workspace revision (ADR 0119): saving
// one, then the other, is never refused and says nothing, since neither changed what the other
// shows. A time zone changed from another window says so in the Time zone section alone.
test('two sections of one Workspace save in turn, and only the one changed elsewhere says so', async () => {
  let stored: SiteWorkspaceResource = { ...workspace, revision: 1 }
  const refused: unknown[] = []
  worker.use(
    revenueSettings,
    handleSiteWorkspacesList(() => HttpResponse.json([stored])),
    handleSiteWorkspacesGet(() => HttpResponse.json(stored)),
    handleSiteWorkspacesUpdate(async ({ request }) => {
      const revision = await revisionOf(request.clone())
      const { name } = (await request.json()) as { name: string }
      if (revision !== stored.revision) {
        refused.push(revision)
        return problem(409, { title: 'Changed', code: 'revision_changed' })
      }
      stored = { ...stored, name, revision: stored.revision + 1 }
      return HttpResponse.json(stored)
    }),
    handleSiteWorkspacesSetTimezone(async ({ request }) => {
      const revision = await revisionOf(request.clone())
      const { timezone } = (await request.json()) as { timezone: string }
      if (revision !== stored.revision) {
        refused.push(revision)
        return problem(409, { title: 'Changed', code: 'revision_changed' })
      }
      stored = { ...stored, timezone, revision: stored.revision + 1 }
      return HttpResponse.json(stored)
    }),
  )
  const { screen } = await renderWithRouter(<GeneralPage />, GENERAL_ROUTE)

  await screen.getByLabelText(/^Workspace name/).fill('Acme Inc')
  await screen.getByRole('button', { name: 'Save' }).first().click()
  await expect.poll(() => stored.revision).toBe(2)
  await screen.getByRole('combobox', { name: 'Time zone' }).click()
  await screen.getByRole('option', { name: 'Europe/Berlin' }).click()
  await screen.getByRole('button', { name: 'Save' }).nth(1).click()

  await expect.poll(() => stored.timezone).toBe('Europe/Berlin')
  expect(refused).toEqual([])
  expect(screen.getByRole('button', { name: 'Load the saved version' }).elements()).toHaveLength(0)

  // Another window changes the time zone; the page reads the Workspace again when it is back.
  stored = { ...stored, timezone: 'Asia/Tokyo', revision: stored.revision + 1 }
  await readAgain()

  await expect
    .element(screen.getByRole('button', { name: 'Load the saved version' }))
    .toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Load the saved version' }).elements()).toHaveLength(1)
  await screen.getByLabelText(/^Workspace name/).fill('Acme Two')
  await screen.getByRole('button', { name: 'Save' }).first().click()
  await expect.poll(() => stored.name).toBe('Acme Two')
  expect(refused).toEqual([])
})
