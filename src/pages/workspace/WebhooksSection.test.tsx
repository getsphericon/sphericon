import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import {
  handleSiteAuditList,
  handleSiteWebhooksCreate,
  handleSiteWebhooksDelete,
  handleSiteWebhooksList,
  handleSiteWebhooksRegenerateSecret,
  handleSiteWebhooksTest,
  handleSiteWebhooksUpdate,
} from '../../generated/site/msw.gen.ts'
import type { SiteWebhookEndpointResource } from '../../generated/site/types.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { WebhooksSection } from './WebhooksSection.tsx'

const WORKSPACE_ID = 'TestWs01'

function endpoint(over: Partial<SiteWebhookEndpointResource> = {}): SiteWebhookEndpointResource {
  return {
    id: '1',
    url: 'https://example.com/hook',
    secret: 'whsec_1',
    eventTypes: [],
    enabled: true,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...over,
  }
}

const list = (items: SiteWebhookEndpointResource[]) =>
  handleSiteWebhooksList({
    body: { items, page: 1, pageSize: 20, totalItems: items.length, totalPages: 1 },
  })

// Without a license the change-history link probes the Audit log and hides itself.
beforeEach(() => {
  worker.use(handleSiteAuditList(() => problem(402)))
})

test('lists endpoints, showing all events and the enabled state', async () => {
  worker.use(
    list([
      endpoint(),
      endpoint({
        id: '2',
        url: 'https://example.com/b',
        eventTypes: ['email.opened'],
        enabled: false,
      }),
    ]),
  )
  const { screen } = await renderWithRouter(<WebhooksSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText('https://example.com/hook')).toBeInTheDocument()
  await expect.element(screen.getByText('All events', { exact: true })).toBeInTheDocument()
  await expect
    .element(screen.getByRole('cell', { name: 'email.opened', exact: true }))
    .toBeInTheDocument()
  await expect.element(screen.getByText('Disabled', { exact: true })).toBeInTheDocument()
})

test('lists a failing endpoint with its last error', async () => {
  worker.use(
    list([
      endpoint({
        failure: {
          consecutiveFailures: 7,
          since: '2026-01-01T00:00:00Z',
          lastFailureAt: '2026-01-02T00:00:00Z',
          kind: 'http_status',
          status: 503,
        },
      }),
    ]),
  )
  const { screen } = await renderWithRouter(<WebhooksSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText('Failing', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText(/HTTP 503: 7 deliveries failed/)).toBeInTheDocument()
})

test('says why an endpoint turned itself off', async () => {
  worker.use(list([endpoint({ enabled: false, disabledReason: 'gone' })]))
  const { screen } = await renderWithRouter(<WebhooksSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText('Receiver answered 410 Gone')).toBeInTheDocument()
})

test('says when an owner turned an endpoint off', async () => {
  worker.use(
    list([
      endpoint({ enabled: false, disabledReason: 'owner', disabledAt: '2026-01-03T00:00:00Z' }),
    ]),
  )
  const { screen } = await renderWithRouter(<WebhooksSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText(/Turned off by a member \(/)).toBeInTheDocument()
})

test('creates an endpoint from the form', async () => {
  const bodies: unknown[] = []
  worker.use(
    list([]),
    handleSiteWebhooksCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(endpoint(), { status: 201 })
    }),
  )
  const { screen } = await renderWithRouter(<WebhooksSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'Add endpoint' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByLabelText(/^Endpoint URL/).fill('  https://example.com/new  ')
  await dialog.getByRole('button', { name: 'Add endpoint' }).click()

  await expect.poll(() => bodies).toEqual([{ url: 'https://example.com/new', eventTypes: [] }])
})

test('toggles an endpoint off', async () => {
  const bodies: unknown[] = []
  worker.use(
    list([endpoint({ eventTypes: ['email.opened'] })]),
    handleSiteWebhooksUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(endpoint({ enabled: false }))
    }),
  )
  const { screen } = await renderWithRouter(<WebhooksSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Disable' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([{ url: 'https://example.com/hook', eventTypes: ['email.opened'], enabled: false }])
})

test('regenerates the secret after confirmation', async () => {
  let regenerated = false
  worker.use(
    list([endpoint()]),
    handleSiteWebhooksRegenerateSecret(() => {
      regenerated = true
      return HttpResponse.json(endpoint({ secret: 'whsec_2' }))
    }),
  )
  const { screen } = await renderWithRouter(<WebhooksSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Regenerate secret' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Regenerate secret' }).click()

  await expect.poll(() => regenerated).toBe(true)
})

test('sends a test event and shows what the receiver answered', async () => {
  const bodies: unknown[] = []
  worker.use(
    list([endpoint()]),
    handleSiteWebhooksTest(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ delivered: false, kind: 'http_status', status: 410 })
    }),
  )
  const { screen } = await renderWithRouter(<WebhooksSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Send test event' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Send' }).click()

  await expect.poll(() => bodies).toEqual([{ eventType: 'contact.created' }])
  await expect.element(screen.getByText('Not delivered: HTTP 410')).toBeInTheDocument()
})

test.each([
  [{ delivered: true, status: 204 }, 'Delivered: the receiver answered HTTP 204'],
  [{ delivered: false, kind: 'timeout' }, 'Not delivered: Timed out'],
  [{ delivered: false }, 'Not delivered: Network error'],
])('shows the outcome of a test event answered with %o', async (answer, message) => {
  worker.use(
    list([endpoint()]),
    handleSiteWebhooksTest(() => HttpResponse.json(answer)),
  )
  const { screen } = await renderWithRouter(<WebhooksSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Send test event' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Send' }).click()

  await expect.element(screen.getByText(message)).toBeInTheDocument()
})

test('deletes an endpoint after confirmation', async () => {
  let deleted = false
  worker.use(
    list([endpoint()]),
    handleSiteWebhooksDelete(() => {
      deleted = true
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(<WebhooksSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Delete' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.poll(() => deleted).toBe(true)
})

test('shows an error alert when the list fails to load', async () => {
  worker.use(handleSiteWebhooksList(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(<WebhooksSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText('Failed to load webhooks').first()).toBeInTheDocument()
})

test('reports a create failure', async () => {
  worker.use(
    list([]),
    handleSiteWebhooksCreate(() => problem(422, { detail: 'url must be https' })),
  )
  const { screen } = await renderWithRouter(<WebhooksSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'Add endpoint' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByLabelText(/^Endpoint URL/).fill('http://x.test')
  await dialog.getByRole('button', { name: 'Add endpoint' }).click()

  await expect.element(screen.getByText('Failed to create webhook')).toBeInTheDocument()
})
