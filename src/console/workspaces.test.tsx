import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleOperatorWorkspacesList } from '../generated/operator/msw.gen.ts'
import type { OperatorWorkspaceResource } from '../generated/operator/types.gen.ts'
import { consoleHomeRoute, consoleWorkspaceRoute } from '../router.tsx'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { worker } from '../test/worker.ts'
import { ConsoleWorkspacesPage } from './workspaces.tsx'

const HOME = routeMount(consoleHomeRoute)

function workspace(over: Partial<OperatorWorkspaceResource> = {}): OperatorWorkspaceResource {
  return { id: '1', slug: 'acme', name: 'Acme', createdAt: '2026-01-01T00:00:00Z', ...over }
}

const page = (items: OperatorWorkspaceResource[]) => ({
  items,
  page: 1,
  pageSize: 25,
  totalItems: items.length,
  totalPages: 1,
})

test('lists workspaces with their suspension state', async () => {
  worker.use(
    handleOperatorWorkspacesList({
      body: page([
        workspace(),
        workspace({
          id: '5',
          slug: 'hooli',
          name: 'Hooli',
          suspension: {
            at: '2026-02-01T00:00:00Z',
            actor: { kind: 'operator', id: 'op-fixture' },
            reason: 'abuse report',
          },
        }),
      ]),
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacesPage />, HOME)

  await expect.element(screen.getByText('acme', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('hooli', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Suspended', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Active', { exact: true })).toBeInTheDocument()
})

test('searching sends the slug to the API', async () => {
  const slugs: (string | null)[] = []
  worker.use(
    handleOperatorWorkspacesList(({ request }) => {
      slugs.push(new URL(request.url).searchParams.get('slug'))
      return HttpResponse.json(page([workspace()]))
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacesPage />, HOME)

  await screen.getByLabelText('Search by slug').fill('hoo')

  await expect.poll(() => slugs).toContain('hoo')
})

test('opening a row navigates to the workspace', async () => {
  worker.use(handleOperatorWorkspacesList({ body: page([workspace({ id: '7' })]) }))
  const { screen, navigate } = await renderWithRouter(<ConsoleWorkspacesPage />, HOME)

  await screen.getByText('acme', { exact: true }).click()

  await expect
    .poll(() => navigate.mock.calls[0]?.[0])
    .toMatchObject({ to: consoleWorkspaceRoute.to, params: { workspaceId: '7' } })
})

test('a failed load shows the error', async () => {
  worker.use(handleOperatorWorkspacesList(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(<ConsoleWorkspacesPage />, HOME)

  await expect.element(screen.getByText('boom')).toBeInTheDocument()
})
