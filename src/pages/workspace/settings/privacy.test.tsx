import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'

import {
  handleSiteContactsErase,
  handleSiteContactsExport,
  handleSiteContactsLookup,
  handleSiteWorkspacesList,
} from '../../../generated/site/msw.gen.ts'
import type {
  SiteContactHoldings,
  SiteWorkspaceResource,
} from '../../../generated/site/types.gen.ts'
import { Route as privacyRoute } from '../../../routes/workspaces/$workspaceId/settings/privacy.tsx'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { PrivacyPage } from './privacy.tsx'

const saved = vi.hoisted((): (string | undefined)[] => [])

vi.mock('file-saver', () => ({
  saveAs: (_data: Blob | string, filename?: string) => {
    saved.push(filename)
  },
}))

const PRIVACY_ROUTE = routeMount(privacyRoute, { workspaceId: 'TestWs01' })

const workspace: SiteWorkspaceResource = {
  id: 'TestWs01',
  name: 'Acme',
  collectKey: 'omck_test_key',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions: ['contacts:erase'],
  mcpUrl: 'https://mcp.sphericon.test/',
  trackerUrl: 'https://t.sphericon.test',
  holds: [],
  sendCap: null,
  role: 'owner',
  createdAt: '2026-01-01T00:00:00Z',
}

const held: SiteContactHoldings = {
  contactHeld: true,
  events: 12,
  deliveryRecords: 3,
  unsubscribes: 1,
  suppressions: 2,
  capturedMessages: 4,
}

async function lookUp(
  screen: Awaited<ReturnType<typeof renderWithRouter>>['screen'],
  email: string,
) {
  await screen.getByLabelText('Email address').fill(email)
  await screen.getByRole('button', { name: 'Look up' }).click()
}

test('contacts:erase looks up an address and sees what is held as counts', async () => {
  const asked: (string | null)[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteContactsLookup(({ request }) => {
      asked.push(new URL(request.url).searchParams.get('email'))
      return Response.json(held)
    }),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)
  await lookUp(screen, '  Alice@Example.com ')

  await expect.element(screen.getByText('Contact: held')).toBeInTheDocument()
  await expect.element(screen.getByText('Events: 12')).toBeInTheDocument()
  await expect.element(screen.getByText('Delivery records: 3')).toBeInTheDocument()
  await expect.element(screen.getByText('Unsubscribes: 1')).toBeInTheDocument()
  await expect.element(screen.getByText('Suppressions: 2')).toBeInTheDocument()
  await expect.element(screen.getByText('Captured messages: 4')).toBeInTheDocument()
  expect(asked).toEqual(['alice@example.com'])
})

test('an address nothing holds says nothing is held', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteContactsLookup({
      body: {
        contactHeld: false,
        events: 0,
        deliveryRecords: 0,
        unsubscribes: 0,
        suppressions: 0,
        capturedMessages: 0,
      },
    }),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)
  await lookUp(screen, 'nobody@example.com')

  await expect.element(screen.getByText('Nothing is held for this address.')).toBeInTheDocument()
})

test('a rejected address shows the problem instead of a result', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteContactsLookup(() => problem(422, { detail: 'not a valid email address' })),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)
  await lookUp(screen, 'not an address')

  await expect.element(screen.getByText('This is not a valid email address.')).toBeInTheDocument()
  await expect.element(screen.getByText('Could not look up the address.')).not.toBeInTheDocument()
})

test('a failed lookup other than a rejected address shows the generic error', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteContactsLookup(() => problem(500, { detail: 'boom' })),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)
  await lookUp(screen, 'alice@example.com')

  await expect.element(screen.getByText('Could not look up the address.')).toBeInTheDocument()
})

test('a found Contact can be exported by the looked-up address', async () => {
  const asked: (string | null)[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteContactsLookup({ body: held }),
    handleSiteContactsExport(({ request }) => {
      asked.push(new URL(request.url).searchParams.get('email'))
      return HttpResponse.json(
        { id: '9' },
        { headers: { 'Content-Disposition': 'attachment; filename="alice.json"' } },
      )
    }),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)
  await lookUp(screen, 'Alice@Example.com')
  await screen.getByRole('button', { name: 'Export data' }).click()

  await expect.poll(() => saved).toEqual(['alice.json'])
  expect(asked).toEqual(['alice@example.com'])
})

test('Export is disabled when the address has no Contact', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteContactsLookup({
      body: {
        contactHeld: false,
        events: 4,
        deliveryRecords: 0,
        unsubscribes: 0,
        suppressions: 0,
        capturedMessages: 0,
      },
    }),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)
  await lookUp(screen, 'ghost@example.com')

  await expect.element(screen.getByText('Contact: none')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Export data' })).toBeDisabled()
})

test('without contacts:erase the section is not offered', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [{ ...workspace, role: 'member', permissions: [] }] }),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)

  await expect.element(screen.getByText(/Only owners and admins/)).toBeInTheDocument()
  await expect.element(screen.getByLabelText('Email address')).not.toBeInTheDocument()
})

const noOptOuts = { unsubscribes: 0, suppressions: 0, capturedMessages: 0 }

test('Erase asks for a confirmation that says what is removed, anonymized and kept, then erases', async () => {
  const erased: (string | null)[] = []
  let lookups = 0
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteContactsLookup(() => {
      lookups += 1
      return Response.json(
        lookups === 1
          ? held
          : { ...held, contactHeld: false, events: 0, deliveryRecords: 0, capturedMessages: 0 },
      )
    }),
    handleSiteContactsErase(({ request }) => {
      erased.push(new URL(request.url).searchParams.get('email'))
      return new Response(null, { status: 204 })
    }),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)
  await lookUp(screen, 'Alice@Example.com')
  await screen.getByRole('button', { name: 'Erase' }).click()

  await expect.element(screen.getByText(/Erasure is permanent/)).toBeInTheDocument()
  await expect.element(screen.getByText(/anonymized/)).toBeInTheDocument()
  await expect
    .element(screen.getByText(/Unsubscribes and suppressions are kept/))
    .toBeInTheDocument()
  await screen.getByRole('dialog').getByRole('button', { name: 'Erase' }).click()

  await expect.element(screen.getByText('Address erased')).toBeInTheDocument()
  await expect.element(screen.getByText('Contact: none')).toBeInTheDocument()
  expect(erased).toEqual(['alice@example.com'])
  await expect.element(screen.getByRole('button', { name: 'Erase' })).toBeDisabled()
})

test('after an Erase the lookup shows only the kept opt-outs and Erase is disabled', async () => {
  let lookups = 0
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteContactsLookup(() => {
      lookups += 1
      return Response.json(
        lookups === 1
          ? held
          : {
              contactHeld: false,
              events: 0,
              deliveryRecords: 0,
              unsubscribes: 1,
              suppressions: 2,
              capturedMessages: 0,
            },
      )
    }),
    handleSiteContactsErase(() => new Response(null, { status: 204 })),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)
  await lookUp(screen, 'alice@example.com')
  await screen.getByRole('button', { name: 'Erase' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Erase' }).click()

  await expect.element(screen.getByText('Contact: none')).toBeInTheDocument()
  await expect.element(screen.getByText('Events: 0')).toBeInTheDocument()
  await expect.element(screen.getByText('Delivery records: 0')).toBeInTheDocument()
  await expect.element(screen.getByText('Unsubscribes: 1')).toBeInTheDocument()
  await expect.element(screen.getByText('Suppressions: 2')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Erase' })).toBeDisabled()
})

test('Erase is enabled for delivery records alone and disabled when only opt-outs remain', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteContactsLookup(({ request }) => {
      const email = new URL(request.url).searchParams.get('email')
      return Response.json(
        email === 'records@example.com'
          ? { contactHeld: false, events: 0, deliveryRecords: 2, ...noOptOuts }
          : {
              contactHeld: false,
              events: 0,
              deliveryRecords: 0,
              unsubscribes: 1,
              suppressions: 1,
              capturedMessages: 0,
            },
      )
    }),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)
  await lookUp(screen, 'records@example.com')
  await expect.element(screen.getByRole('button', { name: 'Erase' })).toBeEnabled()

  await lookUp(screen, 'kept@example.com')
  await expect.element(screen.getByText('Unsubscribes: 1')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Erase' })).toBeDisabled()
})

test('Erase is enabled for captured messages alone, shown as a count', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteContactsLookup({
      body: {
        contactHeld: false,
        events: 0,
        deliveryRecords: 0,
        ...noOptOuts,
        capturedMessages: 2,
      },
    }),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)
  await lookUp(screen, 'tester@example.com')

  await expect.element(screen.getByText('Captured messages: 2')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Erase' })).toBeEnabled()
})

test('a not-found answer says nothing was found and refreshes the lookup', async () => {
  let lookups = 0
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteContactsLookup(() => {
      lookups += 1
      return Response.json(
        lookups === 1
          ? held
          : { ...held, contactHeld: false, events: 0, deliveryRecords: 0, capturedMessages: 0 },
      )
    }),
    handleSiteContactsErase(() => problem(404, { detail: 'nothing found for the address' })),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)
  await lookUp(screen, 'alice@example.com')
  await screen.getByRole('button', { name: 'Erase' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Erase' }).click()

  await expect.element(screen.getByText(/Nothing found for this address/)).toBeInTheDocument()
  await expect.element(screen.getByText('Contact: none')).toBeInTheDocument()
})
