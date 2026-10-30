import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleStaffWorkspacesList } from '../generated/staffee/msw.gen.ts'
import type { StaffWorkspaceResource } from '../generated/staffee/types.gen.ts'
import { Route as consoleWorkspaceRoute } from '../routes/console/_authed/_enterprise/workspaces.$workspaceId.tsx'
import { Route as consoleWorkspacesRoute } from '../routes/console/_authed/_enterprise/workspaces.index.tsx'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { worker } from '../test/worker.ts'
import { ConsoleWorkspacesPage } from './workspaces.tsx'

const HOME = routeMount(consoleWorkspacesRoute)

function workspace(over: Partial<StaffWorkspaceResource> = {}): StaffWorkspaceResource {
  return {
    id: 'AcmeWs01',
    name: 'Acme',
    createdAt: '2026-01-01T00:00:00Z',
    exempt: false,
    warned: false,
    ...over,
  }
}

const page = (items: StaffWorkspaceResource[]) => ({
  items,
  page: 1,
  pageSize: 25,
  totalItems: items.length,
  totalPages: 1,
})

test('lists workspaces with their suspension state', async () => {
  worker.use(
    handleStaffWorkspacesList({
      body: page([
        workspace(),
        workspace({
          id: 'HooliWs5',
          name: 'Hooli',
          suspension: {
            at: '2026-02-01T00:00:00Z',
            actor: { kind: 'staff', id: 'op-fixture' },
            reason: 'abuse report',
          },
        }),
      ]),
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacesPage />, HOME)

  await expect.element(screen.getByText('AcmeWs01', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('HooliWs5', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Suspended', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Active', { exact: true })).toBeInTheDocument()
})

test('searching sends the text to the API as id or name', async () => {
  const searches: (string | null)[] = []
  worker.use(
    handleStaffWorkspacesList(({ request }) => {
      searches.push(new URL(request.url).searchParams.get('search'))
      return HttpResponse.json(page([workspace()]))
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacesPage />, HOME)

  await screen.getByLabelText('Search by id or name').fill('hoo')

  await expect.poll(() => searches).toContain('hoo')
})

test('a warned and an exempt workspace say so next to their state', async () => {
  worker.use(
    handleStaffWorkspacesList({
      body: page([
        workspace({ id: 'GlobxWs2', name: 'Globex', warned: true }),
        workspace({ id: 'KramrWs8', name: 'Kramerica', exempt: true }),
      ]),
    }),
  )

  const { screen } = await renderWithRouter(<ConsoleWorkspacesPage />, HOME)

  await expect.element(screen.getByText('Warned', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Exempt', { exact: true })).toBeInTheDocument()
})

test('choosing a review state sends it to the API', async () => {
  const reviews: (string | null)[] = []
  worker.use(
    handleStaffWorkspacesList(({ request }) => {
      reviews.push(new URL(request.url).searchParams.get('review'))
      return HttpResponse.json(page([workspace()]))
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacesPage />, HOME)

  await screen.getByRole('combobox', { name: 'Review' }).click()
  await screen.getByRole('option', { name: 'Frozen by the detector' }).click()

  await expect.poll(() => reviews).toContain('systemFrozen')
})

test('opening a row navigates to the workspace', async () => {
  worker.use(handleStaffWorkspacesList({ body: page([workspace({ id: 'Seven007' })]) }))
  const { screen, navigate } = await renderWithRouter(<ConsoleWorkspacesPage />, HOME)

  await screen.getByText('Seven007', { exact: true }).click()

  await expect
    .poll(() => navigate.mock.calls[0]?.[0])
    .toMatchObject({ to: consoleWorkspaceRoute.to, params: { workspaceId: 'Seven007' } })
})

test('a failed load shows the error', async () => {
  worker.use(handleStaffWorkspacesList(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(<ConsoleWorkspacesPage />, HOME)

  await expect.element(screen.getByText('boom')).toBeInTheDocument()
})
