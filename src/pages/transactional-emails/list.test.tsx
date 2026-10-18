import { expect, test } from 'vitest'

import { handleSiteTransactionalEmailsList } from '../../generated/site/msw.gen.ts'
import { Route as transactionalEmailsRoute } from '../../routes/workspaces/$slug/transactional-emails/index.tsx'
import { page } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { formatDateTime } from '../../utils/datetime.ts'
import { TransactionalEmailsListPage } from './list.tsx'

const LIST_ROUTE = routeMount(transactionalEmailsRoute, { slug: 'test' })

const SENT = {
  id: '1',
  channel: 'email' as const,
  templateId: '1',
  destination: 'alice@example.com',
  status: 'sent' as const,
  createdAt: '2026-03-04T10:15:00Z',
}
const FAILED = {
  id: '2',
  channel: 'email' as const,
  templateId: '1',
  destination: 'bob@example.com',
  status: 'failed' as const,
  error: 'mailbox full',
  createdAt: '2026-03-05T11:30:00Z',
}

test('lists transactional emails with status, error and sent time', async () => {
  worker.use(handleSiteTransactionalEmailsList({ body: page([SENT, FAILED], 2) }))

  const { screen } = await renderWithRouter(<TransactionalEmailsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Transactional emails')).toBeInTheDocument()
  await expect.element(screen.getByText('alice@example.com')).toBeInTheDocument()
  await expect.element(screen.getByText('bob@example.com')).toBeInTheDocument()
  await expect.element(screen.getByText('Sent', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Failed', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('mailbox full')).toBeInTheDocument()
  await expect.element(screen.getByText(formatDateTime(SENT.createdAt))).toBeInTheDocument()
})

test('shows the empty message when there are no emails', async () => {
  worker.use(handleSiteTransactionalEmailsList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<TransactionalEmailsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('No transactional emails yet')).toBeInTheDocument()
})

test('shows an error alert when the list fails to load', async () => {
  worker.use(handleSiteTransactionalEmailsList(() => problem(500)))

  const { screen } = await renderWithRouter(<TransactionalEmailsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Failed to load transactional emails')).toBeInTheDocument()
})
