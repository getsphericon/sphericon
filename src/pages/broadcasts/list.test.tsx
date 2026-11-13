import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteBroadcastsArchive,
  handleSiteBroadcastsList,
  handleSiteBroadcastsUnarchive,
} from '../../generated/site/msw.gen.ts'
import { Route as broadcastsRoute } from '../../routes/workspaces/$workspaceId/broadcasts/index.tsx'
import { testId } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { BroadcastsListPage } from './list.tsx'

const LIST_ROUTE = routeMount(broadcastsRoute, { workspaceId: 'TestWs01' })
const ARCHIVED_ROUTE = { ...LIST_ROUTE, initialPath: `${LIST_ROUTE.initialPath}?archived=true` }

const STATS = {
  recipientsTotal: 120,
  openedCount: 45,
  revenue: { reportingCurrency: 'USD', orders: 7, amount: 123456 },
}
const DRAFT = {
  id: testId(1),
  name: 'Spring launch',
  channel: 'email',
  status: 'draft',
  stats: STATS,
}
const HELD = {
  id: testId(2),
  name: 'Autumn digest',
  channel: 'email',
  status: 'sending',
  holdReason: 'unverified_domain',
  stats: STATS,
}

test('lists broadcasts with status, hold badge and counters', async () => {
  worker.use(
    handleSiteBroadcastsList(() => HttpResponse.json({ items: [DRAFT, HELD], totalItems: 2 })),
  )

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()
  await expect.element(screen.getByText('Autumn digest')).toBeInTheDocument()
  await expect.element(screen.getByText('Draft')).toBeInTheDocument()
  await expect.element(screen.getByText('Sending', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('On hold')).toBeInTheDocument()
  await expect.element(screen.getByText('45').first()).toBeInTheDocument()
  await expect.element(screen.getByText('$1,234.56').first()).toBeInTheDocument()
  await expect.element(screen.getByText('7', { exact: true }).first()).toBeInTheDocument()
})

test('shows no revenue figure while the reporting currency is unset', async () => {
  const unset = { ...STATS, revenue: { reportingCurrency: null, orders: 0, amount: 0 } }
  worker.use(
    handleSiteBroadcastsList(() =>
      HttpResponse.json({ items: [{ ...DRAFT, stats: unset }], totalItems: 1 }),
    ),
  )

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()
  await expect.element(screen.getByText('$0.00')).not.toBeInTheDocument()
})

test('shows the empty state when there are no broadcasts', async () => {
  worker.use(handleSiteBroadcastsList(() => HttpResponse.json({ items: [], totalItems: 0 })))

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('No broadcasts yet')).toBeInTheDocument()
  // One create action in the empty state, the same label as the header's button.
  await expect
    .element(screen.getByRole('link', { name: 'New broadcast' }).nth(1))
    .toBeInTheDocument()
})

test('shows an error alert when the list fails to load', async () => {
  worker.use(handleSiteBroadcastsList(() => problem(500)))

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Failed to load broadcasts')).toBeInTheDocument()
})

test('New broadcast links to the create page', async () => {
  worker.use(handleSiteBroadcastsList(() => HttpResponse.json({ items: [], totalItems: 0 })))

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)

  await expect
    .element(screen.getByRole('link', { name: 'New broadcast' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/broadcasts/new')
})

test('Report and Edit link with the broadcast id; Edit is only enabled for drafts', async () => {
  worker.use(
    handleSiteBroadcastsList(() => HttpResponse.json({ items: [DRAFT, HELD], totalItems: 2 })),
  )

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'More actions' }).nth(1).click()
  await expect
    .element(screen.getByRole('menuitem', { name: 'Edit' }))
    .toHaveAttribute('data-disabled')
  await screen.getByRole('button', { name: 'More actions' }).nth(1).click()
  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await expect
    .element(screen.getByRole('menuitem', { name: 'Report' }))
    .toHaveAttribute('href', `/workspaces/TestWs01/broadcasts/${testId(1)}/report`)
  await expect
    .element(screen.getByRole('menuitem', { name: 'Edit' }))
    .toHaveAttribute('href', `/workspaces/TestWs01/broadcasts/${testId(1)}/edit`)
})

test('a draft has no Delete: archiving it calls archive and refreshes the list', async () => {
  const archived: string[] = []
  let listFetches = 0
  worker.use(
    handleSiteBroadcastsList(() => {
      listFetches++
      return HttpResponse.json({
        items: archived.length > 0 ? [HELD] : [DRAFT, HELD],
        totalItems: 2,
      })
    }),
    handleSiteBroadcastsArchive(({ params }) => {
      archived.push(params.id)
      return HttpResponse.json({ ...DRAFT, archivedAt: '2026-01-01T00:00:00Z' })
    }),
  )

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await expect.element(screen.getByRole('menuitem', { name: 'Delete' })).not.toBeInTheDocument()
  await screen.getByRole('menuitem', { name: 'Archive' }).click()

  await expect.poll(() => archived).toEqual([testId(1)])
  await expect.element(screen.getByText('Broadcast archived')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.element(screen.getByText('Spring launch')).not.toBeInTheDocument()
})

test('a sending broadcast is work in flight and offers no Archive', async () => {
  worker.use(handleSiteBroadcastsList(() => HttpResponse.json({ items: [HELD], totalItems: 1 })))

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)
  await screen.getByRole('button', { name: 'More actions' }).first().click()

  await expect.element(screen.getByRole('menuitem', { name: 'Report' })).toBeInTheDocument()
  await expect.element(screen.getByRole('menuitem', { name: 'Archive' })).not.toBeInTheDocument()
})

test('the page in the URL is the page of the broadcasts requested', async () => {
  const pages: (string | null)[] = []
  worker.use(
    handleSiteBroadcastsList(({ request }) => {
      pages.push(new URL(request.url).searchParams.get('page'))
      return HttpResponse.json({ items: [DRAFT], totalItems: 25 })
    }),
  )

  await renderWithRouter(<BroadcastsListPage />, {
    ...LIST_ROUTE,
    initialPath: `${LIST_ROUTE.initialPath}?page=2`,
  })

  await expect.poll(() => pages).toEqual(['2'])
})

test('choosing a page of the broadcasts puts it in the URL', async () => {
  worker.use(handleSiteBroadcastsList(() => HttpResponse.json({ items: [DRAFT], totalItems: 25 })))

  const { screen, navigate } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()
  await screen.getByRole('button', { name: '2', exact: true }).click()

  expect(navigate).toHaveBeenCalledWith(expect.objectContaining({ search: expect.any(Function) }))
})

test('a failed archive shows the error toast and keeps the broadcast', async () => {
  worker.use(
    handleSiteBroadcastsList(() => HttpResponse.json({ items: [DRAFT], totalItems: 1 })),
    handleSiteBroadcastsArchive(() => problem(500)),
  )

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)
  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Archive' }).click()

  await expect.element(screen.getByText('Failed to archive broadcast')).toBeInTheDocument()
  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()
})

const SENT = {
  id: testId(3),
  name: 'Winter recap',
  channel: 'email',
  status: 'sent',
  stats: STATS,
}

test('the archived control opens the archived view through the URL', async () => {
  worker.use(handleSiteBroadcastsList(() => HttpResponse.json({ items: [], totalItems: 0 })))

  const { screen, navigate } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)
  await screen.getByText('Archived', { exact: true }).click()

  expect(navigate).toHaveBeenCalledWith(
    expect.objectContaining({ search: { archived: true }, replace: true }),
  )
})

test('the archived view lists archived broadcasts and restores one', async () => {
  const flags: (string | null)[] = []
  const restored: string[] = []
  worker.use(
    handleSiteBroadcastsList(({ request }) => {
      flags.push(new URL(request.url).searchParams.get('archived'))
      return HttpResponse.json({
        items: [{ ...SENT, archivedAt: '2026-01-01T00:00:00Z' }],
        totalItems: 1,
      })
    }),
    handleSiteBroadcastsUnarchive(({ params }) => {
      restored.push(params.id)
      return HttpResponse.json(SENT)
    }),
  )

  const { screen } = await renderWithRouter(<BroadcastsListPage />, ARCHIVED_ROUTE)
  await expect.poll(() => flags).toEqual(['true'])

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Restore' }).click()

  await expect.poll(() => restored).toEqual([testId(3)])
  await expect.element(screen.getByText('Broadcast restored')).toBeInTheDocument()
})

test('archiving a sent broadcast calls archive and refreshes the list', async () => {
  const archived: string[] = []
  let listFetches = 0
  worker.use(
    handleSiteBroadcastsList(() => {
      listFetches++
      return HttpResponse.json({ items: [SENT], totalItems: 1 })
    }),
    handleSiteBroadcastsArchive(({ params }) => {
      archived.push(params.id)
      return HttpResponse.json({ ...SENT, archivedAt: '2026-01-01T00:00:00Z' })
    }),
  )

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)
  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Archive' }).click()

  await expect.poll(() => archived).toEqual([testId(3)])
  await expect.element(screen.getByText('Broadcast archived')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
})

test('each broadcast shows its Channel, and the list filters by Channel through the URL', async () => {
  const channels: (string | null)[] = []
  worker.use(
    handleSiteBroadcastsList(({ request }) => {
      channels.push(new URL(request.url).searchParams.get('channel'))
      return HttpResponse.json({ items: [DRAFT], totalItems: 1 })
    }),
  )

  const { screen, navigate } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)
  await expect.element(screen.getByRole('cell', { name: 'Email' })).toBeInTheDocument()
  await screen.getByRole('combobox', { name: 'Channel' }).click()
  await screen.getByRole('option', { name: 'Email' }).click()

  expect(channels).toEqual([null])
  expect(navigate).toHaveBeenCalledWith(
    expect.objectContaining({ search: { channel: 'email' }, replace: true }),
  )
})

test('a list opened on a Channel asks the API for that Channel only', async () => {
  const channels: (string | null)[] = []
  worker.use(
    handleSiteBroadcastsList(({ request }) => {
      channels.push(new URL(request.url).searchParams.get('channel'))
      return HttpResponse.json({ items: [DRAFT], totalItems: 1 })
    }),
  )

  const { screen } = await renderWithRouter(<BroadcastsListPage />, {
    ...LIST_ROUTE,
    initialPath: `${LIST_ROUTE.initialPath}?channel=email`,
  })

  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()
  expect(channels).toEqual(['email'])
})
