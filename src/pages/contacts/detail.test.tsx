import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { HttpResponse } from 'msw'
import { beforeEach, expect, test, vi } from 'vitest'

import {
  handleSiteContactsDelete,
  handleSiteContactsExport,
  handleSiteContactsGet,
  handleSiteEventsList,
  handleSiteMembershipsList,
  handleSiteUserGetMe,
} from '../../generated/site/msw.gen.ts'
import type {
  SiteContactResource,
  SiteMembershipResource,
  SiteMembershipRole,
} from '../../generated/site/types.gen.ts'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$workspaceId/contacts/index.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithProviders } from '../../test/renderWithProviders.tsx'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ContactDetailPage } from './detail.tsx'

const saved = vi.hoisted((): (string | undefined)[] => [])

vi.mock('file-saver', () => ({
  saveAs: (_data: Blob | string, filename?: string) => {
    saved.push(filename)
  },
}))

const DETAIL_ROUTE = routeMount(contactsDetailRoute, { workspaceId: 'TestWs01', contactId: '9' })
const NOW = '2026-01-01T00:00:00Z'

const ADA: SiteContactResource = {
  id: '9',
  email: 'ada@example.com',
  firstName: 'Ada',
  lastName: 'Lovelace',
  customFields: { plan: 'pro', seats: 3, flags: { beta: true }, nothing: null },
  tags: [],
  createdAt: NOW,
  updatedAt: NOW,
}

// The page reads the current member's role on every render; tests that care override it.
beforeEach(() => {
  worker.use(...currentRoleRoutes('member'))
})

const NO_EVENTS = handleSiteEventsList(() => HttpResponse.json({ items: [], totalItems: 0 }))

test('shows the contact, its custom fields and its events', async () => {
  worker.use(handleSiteContactsGet({ body: ADA }), NO_EVENTS)

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByRole('heading', { name: 'ada@example.com' })).toBeInTheDocument()
  await expect.element(screen.getByText('Ada Lovelace')).toBeInTheDocument()
  await expect.element(screen.getByText('Custom fields')).toBeInTheDocument()
  await expect.element(screen.getByText('pro', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('{"beta":true}')).toBeInTheDocument()
  await expect.element(screen.getByText('No events yet')).toBeInTheDocument()
})

test('lists the contact events and filters them by contact id', async () => {
  let requestedContactId: string | null = null
  worker.use(
    handleSiteContactsGet({ body: { ...ADA, customFields: null } }),
    handleSiteEventsList(({ request }) => {
      requestedContactId = new URL(request.url).searchParams.get('contactId')
      return HttpResponse.json({
        items: [{ id: 'e1', subjectId: 's1', action: 'page.viewed', createdAt: NOW }],
        totalItems: 1,
      })
    }),
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByText('page.viewed')).toBeInTheDocument()
  expect(requestedContactId).toBe('9')
  await expect.element(screen.getByText('Custom fields')).not.toBeInTheDocument()
})

test('falls back to the subject id as the title when there is no email', async () => {
  worker.use(
    handleSiteContactsGet({
      body: { id: '9', subjectId: 'anon-1', tags: [], createdAt: NOW, updatedAt: NOW },
    }),
    NO_EVENTS,
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByRole('heading', { name: 'anon-1' })).toBeInTheDocument()
})

test('shows an error alert when the contact fails to load', async () => {
  worker.use(
    handleSiteContactsGet(() => problem(404, 'Gone')),
    NO_EVENTS,
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByText('Failed to load contact').first()).toBeInTheDocument()
})

test('shows an activity error when the events fail to load', async () => {
  worker.use(
    handleSiteContactsGet({ body: ADA }),
    handleSiteEventsList(() => problem(500)),
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByText('Failed to load activity').first()).toBeInTheDocument()
})

test('moving to another contact resets the events page to the first', async () => {
  const eventRequests: { contactId: string | null; page: string | null }[] = []
  worker.use(
    handleSiteContactsGet(({ params }) =>
      HttpResponse.json(
        params.id === '10' ? { ...ADA, id: '10', email: 'grace@example.com' } : ADA,
      ),
    ),
    handleSiteEventsList(({ request }) => {
      const { searchParams } = new URL(request.url)
      eventRequests.push({
        contactId: searchParams.get('contactId'),
        page: searchParams.get('page'),
      })
      return HttpResponse.json({
        items: [{ id: 'e1', subjectId: 's1', action: 'page.viewed', createdAt: NOW }],
        totalItems: 25,
      })
    }),
  )

  // A real in-memory router (renderWithRouter stubs navigation) so the same page
  // instance sees its contactId param change.
  const rootRoute = createRootRoute()
  const detailRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: DETAIL_ROUTE.path,
    component: ContactDetailPage,
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([detailRoute]),
    history: createMemoryHistory({ initialEntries: [DETAIL_ROUTE.initialPath] }),
  })
  const screen = await renderWithProviders(<RouterProvider router={router} />)

  await expect.element(screen.getByRole('heading', { name: 'ada@example.com' })).toBeInTheDocument()
  await screen.getByRole('button', { name: '2', exact: true }).click()
  await expect.poll(() => eventRequests.at(-1)).toEqual({ contactId: '9', page: '2' })

  await router.navigate({
    to: contactsDetailRoute.to,
    params: { workspaceId: 'TestWs01', contactId: '10' },
  })

  await expect
    .element(screen.getByRole('heading', { name: 'grace@example.com' }))
    .toBeInTheDocument()
  await expect.poll(() => eventRequests.at(-1)).toEqual({ contactId: '10', page: '1' })
})

function currentRoleRoutes(role: SiteMembershipRole) {
  const me = { id: '5', name: 'Me', email: 'me@example.com', emailVerified: true, createdAt: NOW }
  const membership = (userId: string, memberRole: SiteMembershipRole): SiteMembershipResource => ({
    id: `m${userId}`,
    userId,
    email: `u${userId}@example.com`,
    name: `User ${userId}`,
    role: memberRole,
    createdAt: NOW,
  })
  return [
    handleSiteUserGetMe({ body: me }),
    handleSiteMembershipsList({ body: [membership('1', 'owner'), membership('5', role)] }),
  ]
}

test('a member can export a contact but is not offered Erase', async () => {
  worker.use(handleSiteContactsGet({ body: ADA }), NO_EVENTS, ...currentRoleRoutes('member'))

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByRole('button', { name: 'Export data' })).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Erase' })).not.toBeInTheDocument()
})

test('an admin erases a contact only after confirming the irreversible action', async () => {
  const erased: string[] = []
  worker.use(
    handleSiteContactsGet({ body: ADA }),
    NO_EVENTS,
    ...currentRoleRoutes('admin'),
    handleSiteContactsDelete(({ params }) => {
      erased.push(params.id)
      return new HttpResponse(null, { status: 204 })
    }),
  )

  const { screen, navigate } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await screen.getByRole('button', { name: 'Erase' }).click()
  await expect.element(screen.getByRole('dialog').getByText(/cannot be undone/)).toBeInTheDocument()
  expect(erased).toEqual([])

  await screen.getByRole('dialog').getByRole('button', { name: 'Erase' }).click()

  await expect.poll(() => erased).toEqual(['9'])
  await expect
    .poll(() => navigate.mock.calls.at(-1)?.[0])
    .toMatchObject({ to: contactsRoute.to, params: { workspaceId: 'TestWs01' } })
})

test('exporting a contact saves the document under the filename the server suggested', async () => {
  worker.use(
    handleSiteContactsGet({ body: ADA }),
    NO_EVENTS,
    handleSiteContactsExport(() =>
      HttpResponse.json(
        { id: '9' },
        {
          headers: {
            'Content-Disposition':
              'attachment; filename="export.json"; filename*=UTF-8\'\'%D0%90%D0%B4%D0%B0.json',
          },
        },
      ),
    ),
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)
  await screen.getByRole('button', { name: 'Export data' }).click()

  await expect.poll(() => saved).toEqual(['Ада.json'])
})
