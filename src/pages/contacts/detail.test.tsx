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
  handleSiteConfirmationRequestsCreate,
  handleSiteContactsDelete,
  handleSiteContactsExport,
  handleSiteContactsGet,
  handleSiteEngagementTrackingCreate,
  handleSiteEngagementTrackingGet,
  handleSiteEventsList,
  handleSiteIntegrationsList,
  handleSiteSmsConsentsConfirm,
  handleSiteSmsConsentsGet,
  handleSiteSmsConsentsOptOut,
  handleSiteTelegramIssueStartLink,
  handleSiteTelegramListBindings,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type {
  EngagementTrackingConsent,
  Permission,
  SiteContactResource,
  SiteIntegrationResource,
} from '../../generated/site/types.gen.ts'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$workspaceId/contacts/index.tsx'
import { testId, workspace as workspacePayload } from '../../test/payloads.ts'
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

const DETAIL_ROUTE = routeMount(contactsDetailRoute, {
  workspaceId: 'TestWs01',
  contactId: testId(9),
})
const NOW = '2026-01-01T00:00:00Z'

const ADA: SiteContactResource = {
  id: testId(9),
  revision: 1,
  email: 'ada@example.com',
  firstName: 'Ada',
  lastName: 'Lovelace',
  customFields: { plan: 'pro', seats: 3, flags: { beta: true }, nothing: null },
  tags: [],
  createdAt: NOW,
  updatedAt: NOW,
}

// The page reads the signed-in User's Permissions on every render; tests that care override them.
beforeEach(() => {
  worker.use(
    holding([]),
    handleSiteTelegramListBindings({ body: [] }),
    handleSiteSmsConsentsGet({ body: {} }),
    handleSiteEngagementTrackingGet({ body: { destinations: [] } }),
    handleSiteIntegrationsList({ body: [] }),
  )
})

const NO_EVENTS = handleSiteEventsList(() => HttpResponse.json({ items: [], totalItems: 0 }))

test('shows the contact, its custom fields and its events', async () => {
  worker.use(handleSiteContactsGet({ body: ADA }), NO_EVENTS)

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByRole('heading', { name: 'ada@example.com' })).toBeInTheDocument()
  await expect.element(screen.getByText('Ada Lovelace')).toBeInTheDocument()
  await expect.element(screen.getByText('plan', { exact: true })).toBeInTheDocument()
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
  expect(requestedContactId).toBe(testId(9))
})

const eventsBySwitch = (requested: (string | null)[]) =>
  handleSiteEventsList(({ request }) => {
    const includeMachine = new URL(request.url).searchParams.get('includeMachine')
    requested.push(includeMachine)
    const human = { id: 'e2', subjectId: 's1', action: 'email.clicked', createdAt: NOW }
    const machine = { id: 'e1', subjectId: 's1', action: 'email.opened', createdAt: NOW }
    return HttpResponse.json({
      items: includeMachine === 'true' ? [machine, human] : [human],
      totalItems: includeMachine === 'true' ? 2 : 1,
    })
  })

test('asks the API for machine events only when the URL says to show them', async () => {
  const requested: (string | null)[] = []
  worker.use(
    handleSiteContactsGet({ body: { ...ADA, customFields: null } }),
    eventsBySwitch(requested),
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByText('Clicked a link in an email')).toBeInTheDocument()
  await expect.element(screen.getByText('Opened an email')).not.toBeInTheDocument()
  expect(requested.at(-1)).not.toBe('true')
})

test('machine events are shown when the URL asks for them', async () => {
  const requested: (string | null)[] = []
  worker.use(
    handleSiteContactsGet({ body: { ...ADA, customFields: null } }),
    eventsBySwitch(requested),
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, {
    ...DETAIL_ROUTE,
    initialPath: `${DETAIL_ROUTE.initialPath}?machine=true`,
  })

  await expect.element(screen.getByText('Opened an email')).toBeInTheDocument()
  expect(requested.at(-1)).toBe('true')
  await expect.element(screen.getByRole('switch', { name: 'Show machine events' })).toBeChecked()
})

test('the machine events switch puts the choice in the URL', async () => {
  worker.use(handleSiteContactsGet({ body: { ...ADA, customFields: null } }), eventsBySwitch([]))

  const { screen, navigate } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)
  await screen.getByRole('switch', { name: 'Show machine events' }).click()

  expect(navigate).toHaveBeenCalledWith(
    expect.objectContaining({ search: { machine: true }, replace: true }),
  )
})

// A one-pixel GIF: an image that loads without the network, so the photo never falls back.
const PIXEL = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7'

test('shows the city, the locale, the birthday and the photo', async () => {
  worker.use(
    handleSiteContactsGet({
      body: {
        ...ADA,
        city: 'London',
        locale: 'en-GB',
        birthday: '1815-12-10',
        avatar: PIXEL,
      },
    }),
    NO_EVENTS,
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByText('London', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('en-GB', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('December 10, 1815')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('img', { name: 'Ada Lovelace' }))
    .toHaveAttribute('src', PIXEL)
})

test('shows the country by name with its flag', async () => {
  worker.use(handleSiteContactsGet({ body: { ...ADA, country: 'US' } }), NO_EVENTS)

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByText('United States', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByRole('img', { name: 'United States' })).toBeInTheDocument()
})

test('shows the phone stored in E.164 in the readable international format', async () => {
  worker.use(handleSiteContactsGet({ body: { ...ADA, phone: '+49301234567' } }), NO_EVENTS)

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByText('+49 30 1234567', { exact: true })).toBeInTheDocument()
})

test('a contact with no country has no flag', async () => {
  worker.use(handleSiteContactsGet({ body: { ...ADA, country: null } }), NO_EVENTS)

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByText('Country')).toBeInTheDocument()
  await expect.element(screen.getByRole('img')).not.toBeInTheDocument()
})

test('falls back to the subject id as the title when there is no email', async () => {
  worker.use(
    handleSiteContactsGet({
      body: {
        id: testId(9),
        subjectId: 'anon-1',
        tags: [],
        revision: 1,
        createdAt: NOW,
        updatedAt: NOW,
      },
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
        params.id === testId(10) ? { ...ADA, id: testId(10), email: 'grace@example.com' } : ADA,
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
  await expect.poll(() => eventRequests.at(-1)).toEqual({ contactId: testId(9), page: '2' })

  await router.navigate({
    to: contactsDetailRoute.to,
    params: { workspaceId: 'TestWs01', contactId: testId(10) },
  })

  await expect
    .element(screen.getByRole('heading', { name: 'grace@example.com' }))
    .toBeInTheDocument()
  await expect.poll(() => eventRequests.at(-1)).toEqual({ contactId: testId(10), page: '1' })
})

// The Workspace as the signed-in User sees it, holding the given Permissions.
function holding(permissions: Permission[]) {
  return handleSiteWorkspacesList({
    body: [
      {
        ...workspacePayload(),
        postalAddress: '',
        permissions,
        role: 'member',
        createdAt: NOW,
      },
    ],
  })
}

test('contacts:export without contacts:erase offers Export data but not Erase', async () => {
  worker.use(handleSiteContactsGet({ body: ADA }), NO_EVENTS, holding(['contacts:export']))

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByRole('button', { name: 'Export data' })).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Erase' })).not.toBeInTheDocument()
})

test('contacts:erase erases a contact only after confirming the irreversible action', async () => {
  const erased: string[] = []
  worker.use(
    handleSiteContactsGet({ body: ADA }),
    NO_EVENTS,
    holding(['contacts:erase']),
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

  await expect.poll(() => erased).toEqual([testId(9)])
  await expect
    .poll(() => navigate.mock.calls.at(-1)?.[0])
    .toMatchObject({ to: contactsRoute.to, params: { workspaceId: 'TestWs01' } })
})

test('a member sends a confirmation request after confirming, and is told the email is on its way', async () => {
  const asked: string[] = []
  worker.use(
    handleSiteContactsGet({ body: ADA }),
    NO_EVENTS,
    holding([]),
    handleSiteConfirmationRequestsCreate(({ params }) => {
      asked.push(params.contactId)
      return HttpResponse.json({ status: 'accepted' }, { status: 202 })
    }),
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await screen.getByRole('button', { name: 'Send confirmation request' }).click()
  expect(asked).toEqual([])
  await screen
    .getByRole('dialog')
    .getByRole('button', { name: 'Send confirmation request' })
    .click()

  await expect.poll(() => asked).toEqual([testId(9)])
  await expect
    .element(screen.getByText('The confirmation email is on its way.'))
    .toBeInTheDocument()
})

test('a contact with no email has no confirmation request', async () => {
  worker.use(handleSiteContactsGet({ body: { ...ADA, email: null } }), NO_EVENTS, holding([]))

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByRole('heading', { level: 4 })).toBeInTheDocument()
  await expect
    .element(screen.getByRole('button', { name: 'Send confirmation request' }))
    .not.toBeInTheDocument()
})

test('exporting a contact saves the document under the filename the server suggested', async () => {
  worker.use(
    handleSiteContactsGet({ body: ADA }),
    NO_EVENTS,
    holding(['contacts:export']),
    handleSiteContactsExport(() =>
      HttpResponse.json(
        { id: testId(9) },
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

async function heading(contact: SiteContactResource) {
  worker.use(handleSiteContactsGet({ body: contact }), NO_EVENTS)
  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)
  return screen
}

test('falls back from the email to the subject id, the phone and a dash in the heading', async () => {
  const base = { ...ADA, email: null, firstName: null, lastName: null, customFields: null }

  const bySubject = await heading({ ...base, subjectId: 'user-77' })
  await expect.element(bySubject.getByRole('heading', { name: 'user-77' })).toBeInTheDocument()

  const byPhone = await heading({ ...base, phone: '+14155550100' })
  await expect
    .element(byPhone.getByRole('heading', { name: '+1 415 555 0100' }))
    .toBeInTheDocument()

  const bare = await heading(base)
  await expect.element(bare.getByRole('heading', { name: '—' })).toBeInTheDocument()
})

test('lists the tags of the contact', async () => {
  worker.use(
    handleSiteContactsGet({ body: { ...ADA, customFields: null, tags: ['vip', 'beta'] } }),
    NO_EVENTS,
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByText('vip', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('beta', { exact: true })).toBeInTheDocument()
})

const BOT: SiteIntegrationResource = {
  id: testId(9),
  name: 'Acme bot',
  channel: 'telegram',
  provider: 'telegram',
  enabled: true,
  isDefault: false,
  maxPerSecond: null,
  maxPerDay: null,
  providerLimit: {
    perSecond: { limit: 30, source: 'provider' },
    perDay: { limit: null, source: null },
    sentLast24h: 0,
    warnings: [],
  },
  config: { kind: 'telegram', botId: 7000000001, username: 'acme_bot' },
  preset: 'telegram',
  params: {},
  secretsRetiring: [],
  revision: 1,
  hookUrl: 'https://app.test/hooks/omik_nine',
  createdAt: NOW,
  updatedAt: NOW,
}

test('lists the Telegram chats of the contact and copies a start link', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteContactsGet({ body: ADA }),
    NO_EVENTS,
    handleSiteIntegrationsList({ body: [BOT] }),
    handleSiteTelegramListBindings({
      body: [
        {
          id: testId(1),
          integrationId: testId(9),
          integrationName: 'Acme bot',
          botUsername: 'acme_bot',
          boundAt: NOW,
        },
      ],
    }),
    handleSiteTelegramIssueStartLink(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ url: 'https://t.me/acme_bot?start=tok', expiresAt: NOW })
    }),
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)
  await expect.element(screen.getByText('@acme_bot')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Copy Telegram link' }).click()

  await expect.element(screen.getByText(/Start link copied/)).toBeInTheDocument()
  expect(bodies).toEqual([{ integrationId: testId(9) }])
})

const TEXTER: SiteContactResource = { ...ADA, phone: '+14155550810' }

test('shows the SMS consent of the contact phone with its evidence, and its opt-out', async () => {
  worker.use(
    handleSiteContactsGet({ body: TEXTER }),
    NO_EVENTS,
    handleSiteSmsConsentsGet({
      body: {
        phone: '+14155550810',
        confirmation: {
          provenance: 'imported',
          wording: 'Text me offers from Acme',
          source: 'checkout form',
          brand: 'Acme',
          confirmedAt: NOW,
        },
        optOut: { via: 'keyword', optedOutAt: NOW },
      },
    }),
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByRole('heading', { name: 'SMS' })).toBeInTheDocument()
  await expect.element(screen.getByText('Recorded with evidence')).toBeInTheDocument()
  await expect.element(screen.getByText('Text me offers from Acme')).toBeInTheDocument()
  await expect.element(screen.getByText('checkout form')).toBeInTheDocument()
  await expect.element(screen.getByText(/^Opted out of SMS on/)).toBeInTheDocument()
  await expect.element(screen.getByText('How they opted out: Texted STOP.')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Record opt-out' }).query()).toBeNull()
})

test('records SMS consent with its evidence and an SMS opt-out', async () => {
  const consents: unknown[] = []
  let optOuts = 0
  worker.use(
    handleSiteContactsGet({ body: TEXTER }),
    NO_EVENTS,
    handleSiteSmsConsentsGet({ body: { phone: '+14155550810' } }),
    handleSiteSmsConsentsConfirm(async ({ request }) => {
      consents.push(await request.json())
      return HttpResponse.json({ phone: '+14155550810' })
    }),
    handleSiteSmsConsentsOptOut(() => {
      optOuts += 1
      return HttpResponse.json({
        phone: '+14155550810',
        optOut: { via: 'manual', optedOutAt: NOW },
      })
    }),
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)
  await expect.element(screen.getByText(/^No SMS consent recorded/)).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Record consent' }).click()
  await screen.getByLabelText(/^Consent wording/).fill('  Text me offers  ')
  await screen.getByLabelText(/^Source/).fill('phone call')
  await screen.getByLabelText(/^Brand/).fill('Acme')
  await screen.getByRole('dialog').getByRole('button', { name: 'Record consent' }).click()
  await expect.element(screen.getByText('SMS consent recorded.')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Record opt-out' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Record opt-out' }).click()

  await expect.element(screen.getByText('SMS opt-out recorded.')).toBeInTheDocument()
  expect(consents).toEqual([{ wording: 'Text me offers', source: 'phone call', brand: 'Acme' }])
  expect(optOuts).toBe(1)
})

test('a consent texted with START carries no evidence, so its evidence rows show a dash', async () => {
  worker.use(
    handleSiteContactsGet({ body: TEXTER }),
    NO_EVENTS,
    handleSiteSmsConsentsGet({
      body: {
        phone: '+14155550810',
        confirmation: { provenance: 'keyword', confirmedAt: NOW },
      },
    }),
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByRole('row', { name: 'How Texted START' })).toBeInTheDocument()
  await expect.element(screen.getByRole('row', { name: 'Consent wording —' })).toBeInTheDocument()
  await expect.element(screen.getByRole('row', { name: 'Source —' })).toBeInTheDocument()
  await expect.element(screen.getByRole('row', { name: 'Brand —' })).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Record opt-out' })).toBeInTheDocument()
})

const ADA_TRACKING: EngagementTrackingConsent = {
  channel: 'email',
  destination: 'ada@example.com',
  state: 'default',
  effective: 'track',
}

function tracking(consent: Partial<EngagementTrackingConsent>) {
  return handleSiteEngagementTrackingGet({
    body: { destinations: [{ ...ADA_TRACKING, ...consent }] },
  })
}

test('an address with no tracking consent shows the default and what it means now', async () => {
  worker.use(handleSiteContactsGet({ body: ADA }), NO_EVENTS, tracking({ effective: 'dont_track' }))

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect
    .element(screen.getByRole('heading', { name: 'Engagement tracking' }))
    .toBeInTheDocument()
  await expect.element(screen.getByText('Default: not tracked')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Refuse tracking' })).toBeEnabled()
  await expect.element(screen.getByRole('button', { name: 'Grant tracking' })).toBeEnabled()
})

test('a granted address offers only the refusal', async () => {
  worker.use(
    handleSiteContactsGet({ body: ADA }),
    NO_EVENTS,
    tracking({ state: 'granted', provenance: 'imported', effective: 'track', updatedAt: NOW }),
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByText('Granted')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Refuse tracking' })).toBeEnabled()
  expect(screen.getByRole('button', { name: 'Grant tracking' }).query()).toBeNull()
})

test('a refused address says who refused it and disables Grant with the reason', async () => {
  worker.use(
    handleSiteContactsGet({ body: ADA }),
    NO_EVENTS,
    tracking({ state: 'denied', provenance: 'manual', effective: 'dont_track', updatedAt: NOW }),
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)
  await screen.getByRole('button', { name: 'Refused' }).click()

  await expect
    .element(screen.getByText('Recorded by a member of the workspace.'))
    .toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Grant tracking' })).toBeDisabled()
  await expect.element(screen.getByText('Only the person can lift a refusal.')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Refuse tracking' }).query()).toBeNull()
})

test('records a refusal of tracking once the address is typed, saying what it deletes', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteContactsGet({ body: ADA }),
    NO_EVENTS,
    tracking({}),
    handleSiteEngagementTrackingCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({
        outcome: 'applied',
        consent: {
          ...ADA_TRACKING,
          state: 'denied',
          provenance: 'manual',
          effective: 'dont_track',
        },
      })
    }),
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)
  await screen.getByRole('button', { name: 'Refuse tracking' }).click()
  const dialog = screen.getByRole('dialog')
  await expect
    .element(dialog.getByText(/opens, clicks and order credits already recorded/))
    .toBeInTheDocument()
  const confirm = dialog.getByRole('button', { name: 'Refuse tracking' })
  await expect.element(confirm).toBeDisabled()
  await dialog.getByLabelText('Type ada@example.com to confirm').fill('ada@example.com')
  await confirm.click()

  await expect.element(screen.getByText('Refusal of tracking recorded.')).toBeInTheDocument()
  expect(bodies).toEqual([{ channel: 'email', destination: 'ada@example.com', state: 'denied' }])
})

test('a grant the server refuses over a refusal recorded meanwhile says why and reads it', async () => {
  let reads = 0
  worker.use(
    handleSiteContactsGet({ body: ADA }),
    NO_EVENTS,
    handleSiteEngagementTrackingGet(() => {
      reads += 1
      const standing: Partial<EngagementTrackingConsent> =
        reads === 1 ? {} : { state: 'denied', provenance: 'imported', effective: 'dont_track' }
      return HttpResponse.json({ destinations: [{ ...ADA_TRACKING, ...standing }] })
    }),
    handleSiteEngagementTrackingCreate(() =>
      problem(409, {
        title: 'Conflict',
        detail: 'the destination refused tracking, and only the person can lift a refusal',
      }),
    ),
  )

  const { screen } = await renderWithRouter(<ContactDetailPage />, DETAIL_ROUTE)
  await screen.getByRole('button', { name: 'Grant tracking' }).click()

  await expect
    .element(screen.getByText('Could not record the tracking consent'))
    .toBeInTheDocument()
  await expect.element(screen.getByText(/only the person can lift a refusal/)).toBeInTheDocument()
})
