import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteCapturedMessagesDeleteAll,
  handleSiteCapturedMessagesList,
} from '../../generated/site/msw.gen.ts'
import type { SiteCapturedMessageSummary } from '../../generated/site/types.gen.ts'
import { Route as capturedMessagesRoute } from '../../routes/workspaces/$workspaceId/captured-messages/index.tsx'
import { page } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { formatDateTime } from '../../utils/datetime.ts'
import { CapturedMessagesListPage } from './list.tsx'

const LIST_ROUTE = routeMount(capturedMessagesRoute, { workspaceId: 'TestWs01' })

const WELCOME: SiteCapturedMessageSummary = {
  id: '1',
  integrationId: '5',
  outboundMessageId: '9',
  surface: 'transactional',
  sendingSource: null,
  recipient: 'alice@example.com',
  from: 'hi@acme.com',
  fromName: 'Acme',
  subject: 'Welcome aboard',
  fromUnverified: false,
  createdAt: '2026-03-04T10:15:00Z',
}
const NEWSLETTER: SiteCapturedMessageSummary = {
  ...WELCOME,
  id: '2',
  surface: 'broadcast',
  recipient: 'bob@example.com',
  subject: 'March news',
  fromUnverified: true,
  createdAt: '2026-03-05T11:30:00Z',
}

test('lists captured messages with recipient, subject, surface and time', async () => {
  worker.use(handleSiteCapturedMessagesList({ body: page([NEWSLETTER, WELCOME], 2) }))

  const { screen } = await renderWithRouter(<CapturedMessagesListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Captured messages').first()).toBeInTheDocument()
  await expect.element(screen.getByText('alice@example.com')).toBeInTheDocument()
  await expect.element(screen.getByText('March news')).toBeInTheDocument()
  await expect.element(screen.getByText('Broadcast', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText(formatDateTime(WELCOME.createdAt))).toBeInTheDocument()
  await expect.element(screen.getByText('From not verified')).toBeInTheDocument()
})

test('shows the empty message when nothing was captured', async () => {
  worker.use(handleSiteCapturedMessagesList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<CapturedMessagesListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('No captured messages yet')).toBeInTheDocument()
})

test('shows an error alert when the list fails to load', async () => {
  worker.use(handleSiteCapturedMessagesList(() => problem(500)))

  const { screen } = await renderWithRouter(<CapturedMessagesListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Failed to load captured messages')).toBeInTheDocument()
})

test('deletes all captured messages after a confirmation', async () => {
  let deleted = false
  worker.use(
    handleSiteCapturedMessagesList({ body: page([WELCOME], 1) }),
    handleSiteCapturedMessagesDeleteAll(() => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(<CapturedMessagesListPage />, LIST_ROUTE)

  await screen.getByRole('button', { name: 'Delete all' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete all' }).click()

  await expect.poll(() => deleted).toBe(true)
})
