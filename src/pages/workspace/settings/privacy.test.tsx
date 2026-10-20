import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'

import {
  handleSiteContactsExport,
  handleSiteContactsLookup,
  handleSiteWorkspacesList,
} from '../../../generated/site/msw.gen.ts'
import type { SiteContactLookup } from '../../../generated/site/types.gen.ts'
import { Route as privacyRoute } from '../../../routes/workspaces/$slug/settings/privacy.tsx'
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

const PRIVACY_ROUTE = routeMount(privacyRoute, { slug: 'test' })

const workspace = {
  id: '1',
  name: 'Acme',
  slug: 'test',
  collectKey: 'omck_test_key',
  postalAddress: '',
  accountLinkAvailable: false,
  mcpUrl: 'https://mcp.sphericon.test/',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

const held: SiteContactLookup = {
  contactHeld: true,
  events: 12,
  deliveryRecords: 3,
  unsubscribes: 1,
  suppressions: 2,
}

async function lookUp(
  screen: Awaited<ReturnType<typeof renderWithRouter>>['screen'],
  email: string,
) {
  await screen.getByLabelText('Email address').fill(email)
  await screen.getByRole('button', { name: 'Look up' }).click()
}

test('an owner looks up an address and sees what is held as counts', async () => {
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
  expect(asked).toEqual(['alice@example.com'])
})

test('an address nothing holds says nothing is held', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteContactsLookup({
      body: { contactHeld: false, events: 0, deliveryRecords: 0, unsubscribes: 0, suppressions: 0 },
    }),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)
  await lookUp(screen, 'nobody@example.com')

  await expect.element(screen.getByText('Nothing is held for this address.')).toBeInTheDocument()
})

test('a rejected address shows the problem instead of a result', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteContactsLookup(() => problem(400, { detail: 'invalid email address' })),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)
  await lookUp(screen, 'not an address')

  await expect.element(screen.getByText('Could not look up the address.')).toBeInTheDocument()
})

test('an admin sees the section', async () => {
  worker.use(handleSiteWorkspacesList({ body: [{ ...workspace, role: 'admin' as const }] }))

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)

  await expect.element(screen.getByLabelText('Email address')).toBeInTheDocument()
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
      body: { contactHeld: false, events: 4, deliveryRecords: 0, unsubscribes: 0, suppressions: 0 },
    }),
  )

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)
  await lookUp(screen, 'ghost@example.com')

  await expect.element(screen.getByText('Contact: none')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Export data' })).toBeDisabled()
})

test('a plain member is not offered the section', async () => {
  worker.use(handleSiteWorkspacesList({ body: [{ ...workspace, role: 'member' as const }] }))

  const { screen } = await renderWithRouter(<PrivacyPage />, PRIVACY_ROUTE)

  await expect.element(screen.getByText(/Only owners and admins/)).toBeInTheDocument()
  await expect.element(screen.getByLabelText('Email address')).not.toBeInTheDocument()
})
