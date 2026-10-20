import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteTagsDelete, handleSiteTagsList } from '../../generated/site/msw.gen.ts'
import { Route as tagsRoute } from '../../routes/workspaces/$workspaceId/tags/index.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { TagsListPage } from './list.tsx'

const LIST_ROUTE = routeMount(tagsRoute, { workspaceId: 'TestWs01' })

const VIP = { id: '1', name: 'vip', createdAt: TIMESTAMPS.createdAt }
const TRIAL = { id: '2', name: 'trial', createdAt: TIMESTAMPS.createdAt }

test('lists the workspace tags', async () => {
  worker.use(handleSiteTagsList({ body: page([VIP, TRIAL], 2) }))

  const { screen } = await renderWithRouter(<TagsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('vip')).toBeInTheDocument()
  await expect.element(screen.getByText('trial')).toBeInTheDocument()
})

test('deleting a tag asks for confirmation, deletes it and refreshes the list', async () => {
  const deleted: string[] = []
  worker.use(
    handleSiteTagsList(() =>
      HttpResponse.json({
        items: deleted.length > 0 ? [TRIAL] : [VIP, TRIAL],
        totalItems: 2,
      }),
    ),
    handleSiteTagsDelete(({ params }) => {
      deleted.push(params.id)
      return new HttpResponse(null, { status: 204 })
    }),
  )

  const { screen } = await renderWithRouter(<TagsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('vip')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'Delete' }).first().click()
  expect(deleted).toEqual([])
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.poll(() => deleted).toEqual(['1'])
  await expect.element(screen.getByText('Tag deleted')).toBeInTheDocument()
  await expect.element(screen.getByText('vip')).not.toBeInTheDocument()
})

test('a tag a segment or an automation names is not deleted and they are named', async () => {
  worker.use(
    handleSiteTagsList({ body: page([VIP], 1) }),
    handleSiteTagsDelete(() =>
      problem(409, {
        usedBy: [
          { kind: 'segment', id: '3', name: 'VIP customers' },
          { kind: 'automation', id: '4', name: 'Tag on engagement' },
        ],
      }),
    ),
  )

  const { screen } = await renderWithRouter(<TagsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('vip')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'Delete' }).first().click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.element(screen.getByText('Failed to delete tag')).toBeInTheDocument()
  await expect.element(screen.getByText(/Segment "VIP customers"/)).toBeInTheDocument()
  await expect.element(screen.getByText(/Automation "Tag on engagement"/)).toBeInTheDocument()
  await expect.element(screen.getByText('vip')).toBeInTheDocument()
})
