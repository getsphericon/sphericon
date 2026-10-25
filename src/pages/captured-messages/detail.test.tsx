import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteCapturedMessagesDelete,
  handleSiteCapturedMessagesGet,
} from '../../generated/site/msw.gen.ts'
import type { SiteCapturedMessageResource } from '../../generated/site/types.gen.ts'
import { Route as capturedMessageRoute } from '../../routes/workspaces/$workspaceId/captured-messages/$messageId/index.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { CapturedMessagePage } from './detail.tsx'

const MESSAGE_ROUTE = routeMount(capturedMessageRoute, { workspaceId: 'TestWs01', messageId: '1' })

const HTML = '<p>Hello <a href="https://t.test/e/c/abc">click</a></p>'

const MESSAGE: SiteCapturedMessageResource = {
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
  html: HTML,
  text: 'Hello plain',
  headers: [{ name: 'List-Unsubscribe', value: '<https://t.test/e/u/xyz>' }],
  links: ['https://t.test/e/c/abc', 'https://t.test/e/confirm/def'],
}

const serve = () => worker.use(handleSiteCapturedMessagesGet({ body: MESSAGE }))

test('renders the message in a frame with no same-origin allowance and no script', async () => {
  serve()
  const { screen } = await renderWithRouter(<CapturedMessagePage />, MESSAGE_ROUTE)

  await expect.element(screen.getByText('Welcome aboard').first()).toBeInTheDocument()
  const frame = screen.getByTitle('Rendered message')
  await expect.element(frame).toBeInTheDocument()
  const element = frame.element()
  expect(element.getAttribute('sandbox')).toBe('')
  expect(element.getAttribute('srcdoc')).toBe(HTML)
})

test('shows the HTML, text and headers of the message in tabs', async () => {
  serve()
  const { screen } = await renderWithRouter(<CapturedMessagePage />, MESSAGE_ROUTE)

  await screen.getByRole('tab', { name: 'HTML' }).click()
  await expect.element(screen.getByText(HTML)).toBeInTheDocument()

  await screen.getByRole('tab', { name: 'Text' }).click()
  await expect.element(screen.getByText('Hello plain')).toBeInTheDocument()

  await screen.getByRole('tab', { name: 'Headers' }).click()
  await expect.element(screen.getByText('List-Unsubscribe')).toBeInTheDocument()
  await expect.element(screen.getByText('<https://t.test/e/u/xyz>')).toBeInTheDocument()
})

test('opens a link of the message in a new tab without an opener', async () => {
  serve()
  const { screen } = await renderWithRouter(<CapturedMessagePage />, MESSAGE_ROUTE)

  await screen.getByRole('tab', { name: 'Links' }).click()

  const link = screen.getByRole('link', { name: 'https://t.test/e/c/abc' }).element()
  expect(link.getAttribute('href')).toBe('https://t.test/e/c/abc')
  expect(link.getAttribute('target')).toBe('_blank')
  expect(link.getAttribute('rel')).toContain('noopener')
  await expect
    .element(screen.getByRole('link', { name: 'https://t.test/e/confirm/def' }))
    .toBeInTheDocument()
})

test('deletes the message after a confirmation', async () => {
  let deleted = false
  serve()
  worker.use(
    handleSiteCapturedMessagesDelete(() => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(<CapturedMessagePage />, MESSAGE_ROUTE)

  await screen.getByRole('button', { name: 'Delete' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.poll(() => deleted).toBe(true)
})

test('shows an error alert when the message fails to load', async () => {
  worker.use(handleSiteCapturedMessagesGet(() => problem(404)))
  const { screen } = await renderWithRouter(<CapturedMessagePage />, MESSAGE_ROUTE)

  await expect.element(screen.getByText('Failed to load the captured message')).toBeInTheDocument()
})
