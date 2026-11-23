import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteSkippedEntriesList } from '../../generated/site/msw.gen.ts'
import { Route as skippedEntriesRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/skipped-entries.tsx'
import { page, testId } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { SkippedEntriesPage } from './SkippedEntries.tsx'

const ROUTE = routeMount(skippedEntriesRoute, { workspaceId: 'TestWs01', automationId: testId(1) })

// Two skipped entries of two contacts; the API filters them by the contactId it is asked for.
const ENTRIES = [
  {
    id: testId(1),
    automationId: testId(1),
    contactId: testId(7),
    reason: 'once' as const,
    eventSourceId: 'evt-7',
    occurredAt: '2026-03-16T10:00:00Z',
    createdAt: '2026-03-16T10:00:01Z',
  },
  {
    id: testId(2),
    automationId: testId(1),
    contactId: testId(8),
    reason: 'active_enrollment' as const,
    eventSourceId: 'evt-8',
    occurredAt: '2026-03-15T10:00:00Z',
    createdAt: '2026-03-15T10:00:01Z',
  },
]

// Every search the page navigated to, each worked out from the search it was on.
type Navigate = Awaited<ReturnType<typeof renderWithRouter>>['navigate']
const searches = (navigate: Navigate, from: Record<string, unknown>) =>
  navigate.mock.calls.map(([options]) =>
    typeof options.search === 'function' ? options.search(from) : options.search,
  )

function serveEntries() {
  worker.use(
    handleSiteSkippedEntriesList(({ request }) => {
      const contactId = new URL(request.url).searchParams.get('contactId')
      const items = ENTRIES.filter((e) => contactId === null || e.contactId === contactId)
      return HttpResponse.json(page(items, items.length))
    }),
  )
}

test('lists the skipped entries with their reasons, each contact a link to the contact', async () => {
  serveEntries()

  const { screen } = await renderWithRouter(<SkippedEntriesPage />, ROUTE)

  await expect.element(screen.getByText('Enrolled before')).toBeInTheDocument()
  await expect.element(screen.getByText('An enrollment was active')).toBeInTheDocument()
  await expect.element(screen.getByText('2 skipped entries')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: testId(7) }))
    .toHaveAttribute('href', `/workspaces/TestWs01/contacts/${testId(7)}`)
})

test('the contact in the URL search filters the list', async () => {
  serveEntries()

  const { screen } = await renderWithRouter(<SkippedEntriesPage />, {
    ...ROUTE,
    initialPath: `${ROUTE.initialPath}?contactId=${testId(8)}`,
  })

  await expect.element(screen.getByText('An enrollment was active')).toBeInTheDocument()
  await expect.element(screen.getByText('Enrolled before')).not.toBeInTheDocument()
  await expect.element(screen.getByRole('textbox', { name: 'Contact ID' })).toHaveValue(testId(8))
})

test('typing a contact id puts it in the URL search, from the first page', async () => {
  serveEntries()

  const { screen, navigate } = await renderWithRouter(<SkippedEntriesPage />, ROUTE)
  await expect.element(screen.getByText('Enrolled before')).toBeInTheDocument()
  await screen.getByRole('textbox', { name: 'Contact ID' }).fill(` ${testId(8)} `)

  await expect
    .poll(() => searches(navigate, { page: 2 }))
    .toContainEqual({ contactId: testId(8), page: undefined })
})

test('a contact with no skipped entries offers to clear the filter', async () => {
  serveEntries()

  const { screen } = await renderWithRouter(<SkippedEntriesPage />, {
    ...ROUTE,
    initialPath: `${ROUTE.initialPath}?contactId=${testId(9)}`,
  })

  await expect.element(screen.getByText('No skipped entries of this contact')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Clear filter' })).toBeInTheDocument()
})

test('an automation that skipped no entry says so, with no filter to clear', async () => {
  worker.use(handleSiteSkippedEntriesList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<SkippedEntriesPage />, ROUTE)

  await expect.element(screen.getByText('No entries were skipped')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Clear filter' })).not.toBeInTheDocument()
})

test('Clear filter empties the field and drops the contact from the URL search', async () => {
  serveEntries()

  const { screen, navigate } = await renderWithRouter(<SkippedEntriesPage />, {
    ...ROUTE,
    initialPath: `${ROUTE.initialPath}?contactId=${testId(9)}`,
  })
  await screen.getByRole('button', { name: 'Clear filter' }).click()

  await expect.element(screen.getByRole('textbox', { name: 'Contact ID' })).toHaveValue('')
  expect(searches(navigate, { contactId: testId(9) })).toContainEqual({
    contactId: undefined,
    page: undefined,
  })
})

test('choosing a row opens its contact', async () => {
  serveEntries()

  const { screen, navigate } = await renderWithRouter(<SkippedEntriesPage />, ROUTE)
  await screen.getByText('Enrolled before').click()

  expect(navigate).toHaveBeenCalledWith(
    expect.objectContaining({
      to: '/workspaces/$workspaceId/contacts/$contactId',
      params: { workspaceId: 'TestWs01', contactId: testId(7) },
    }),
  )
})

test('going back to another contact in the URL search puts it in the field', async () => {
  serveEntries()

  const { screen, router } = await renderWithRouter(<SkippedEntriesPage />, {
    ...ROUTE,
    initialPath: `${ROUTE.initialPath}?contactId=${testId(8)}`,
  })
  await expect.element(screen.getByRole('textbox', { name: 'Contact ID' })).toHaveValue(testId(8))
  router.history.push(`${ROUTE.initialPath}?contactId=${testId(7)}`)

  await expect.element(screen.getByRole('textbox', { name: 'Contact ID' })).toHaveValue(testId(7))
  await expect.element(screen.getByText('Enrolled before')).toBeInTheDocument()
})
