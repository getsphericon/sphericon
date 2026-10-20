import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteWorkspacesCreate,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type { SiteWorkspaceResource } from '../../generated/site/types.gen.ts'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'
import { Route as newWorkspaceRoute } from '../../routes/workspaces/new.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { CreateWorkspacePage } from './create.tsx'

const MOUNT = routeMount(newWorkspaceRoute)

const existing: SiteWorkspaceResource = {
  id: 'TestWs01',
  name: 'Acme',
  collectKey: 'omck_test_key',
  postalAddress: '',
  accountLinkAvailable: false,
  mcpUrl: 'https://sphericon.test',
  holds: [],
  role: 'owner',
  createdAt: '2026-01-01T00:00:00Z',
}

test('a User with no workspace creates one by name and lands in it', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [] }),
    handleSiteWorkspacesCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ ...existing, id: 'NewWs001', name: 'Brand' }, { status: 201 })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<CreateWorkspacePage />, MOUNT)

  await expect.element(screen.getByText(/You are not in a workspace yet/)).toBeInTheDocument()
  await expect.element(screen.getByRole('link', { name: 'Cancel' })).not.toBeInTheDocument()
  await screen.getByLabelText(/^Workspace name/).fill('  Brand  ')
  await screen.getByRole('button', { name: 'Create workspace' }).click()

  await expect.poll(() => bodies).toEqual([{ name: 'Brand' }])
  await expect
    .poll(() => navigate.mock.calls)
    .toContainEqual([{ to: overviewRoute.to, params: { workspaceId: 'NewWs001' } }])
})

test('a User with workspaces can cancel back to the app', async () => {
  worker.use(handleSiteWorkspacesList({ body: [existing] }))
  const { screen } = await renderWithRouter(<CreateWorkspacePage />, MOUNT)

  await expect.element(screen.getByText(/Create another workspace/)).toBeInTheDocument()
  await expect.element(screen.getByRole('link', { name: 'Cancel' })).toBeInTheDocument()
})

test('an unverified User is told to verify the email first', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [] }),
    handleSiteWorkspacesCreate(() => problem(403, { code: 'email_verification_required' })),
  )
  const { screen } = await renderWithRouter(<CreateWorkspacePage />, MOUNT)

  await screen.getByLabelText(/^Workspace name/).fill('Brand')
  await screen.getByRole('button', { name: 'Create workspace' }).click()

  await expect
    .element(screen.getByText('Verify your email address before creating a workspace.'))
    .toBeInTheDocument()
})
