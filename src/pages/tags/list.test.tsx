import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteTagsDelete, handleSiteTagsList } from '../../generated/site/msw.gen.ts'
import { Route as tagsRoute } from '../../routes/workspaces/$workspaceId/tags/index.tsx'
import { page } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { TagsListPage } from './list.tsx'

const LIST_ROUTE = routeMount(tagsRoute, { workspaceId: 'TestWs01' })

const VIP = { id: '1', name: 'vip', createdAt: '2026-01-01T00:00:00Z' }
const TRIAL = { id: '2', name: 'trial', createdAt: '2026-01-02T00:00:00Z' }

test('lists the workspace tags', async () => {
  worker.use(handleSiteTagsList({ body: page([VIP, TRIAL], 2) }))

  const { screen } = await renderWithRouter(<TagsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('vip')).toBeInTheDocument()
  await expect.element(screen.getByText('trial')).toBeInTheDocument()
})

test('shows the empty state when there are no tags', async () => {
  worker.use(handleSiteTagsList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<TagsListPage />, LIST_ROUTE)

  await expect
    .element(screen.getByText('Tags appear here once a contact is tagged.'))
    .toBeInTheDocument()
})

test('shows an error alert when the list fails to load', async () => {
  worker.use(handleSiteTagsList(() => problem(500)))

  const { screen } = await renderWithRouter(<TagsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Failed to load tags')).toBeInTheDocument()
})

test('deleting a tag asks for confirmation, deletes it and refreshes the list', async () => {
  const deleted: string[] = []
  let listFetches = 0
  worker.use(
    handleSiteTagsList(() => {
      listFetches++
      return HttpResponse.json({
        items: deleted.length > 0 ? [TRIAL] : [VIP, TRIAL],
        totalItems: 2,
      })
    }),
    handleSiteTagsDelete(({ params }) => {
      deleted.push(params.id)
      return new HttpResponse(null, { status: 204 })
    }),
  )

  const { screen } = await renderWithRouter(<TagsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('vip')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Delete' }).click()
  expect(deleted).toEqual([])
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.poll(() => deleted).toEqual(['1'])
  await expect.element(screen.getByText('Tag deleted')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.element(screen.getByText('vip')).not.toBeInTheDocument()
})

test('a tag opens the contacts that carry it', async () => {
  worker.use(handleSiteTagsList({ body: page([VIP, TRIAL], 2) }))

  const { screen, navigate } = await renderWithRouter(<TagsListPage />, LIST_ROUTE)
  await screen.getByText('vip').click()

  expect(navigate).toHaveBeenCalledWith(
    expect.objectContaining({
      to: '/workspaces/$workspaceId/contacts',
      params: { workspaceId: 'TestWs01' },
      search: { tag: 'vip' },
    }),
  )
})
