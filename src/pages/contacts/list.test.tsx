import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteContactsList, handleSiteTagsList } from '../../generated/site/msw.gen.ts'
import { Route as contactsRoute } from '../../routes/workspaces/$workspaceId/contacts/index.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ContactsListPage } from './list.tsx'

const LIST_ROUTE = routeMount(contactsRoute, { workspaceId: 'TestWs01' })

// Every search the page navigated to, each worked out from the search it was on: a filter updates
// its own key of the search and leaves the others.
type Navigate = Awaited<ReturnType<typeof renderWithRouter>>['navigate']
const searches = (navigate: Navigate, from: Record<string, unknown>) =>
  navigate.mock.calls.map(([options]) =>
    typeof options?.search === 'function' ? options.search(from) : options?.search,
  )

const ALICE = {
  id: '1',
  email: 'alice@example.com',
  firstName: 'Alice',
  lastName: 'Smith',
  tags: [],
  ...TIMESTAMPS,
}
const BOB = { id: '2', email: 'bob@example.com', tags: [], ...TIMESTAMPS }

test('lists the workspace contacts', async () => {
  worker.use(handleSiteContactsList({ body: page([ALICE, BOB], 2) }))

  const { screen } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('alice@example.com')).toBeInTheDocument()
  await expect.element(screen.getByText('bob@example.com')).toBeInTheDocument()
  await expect.element(screen.getByText('Alice Smith', { exact: true })).toBeInTheDocument()
})

test('shows the country flag of a contact next to its name', async () => {
  worker.use(handleSiteContactsList({ body: page([{ ...ALICE, country: 'GB' }, BOB], 2) }))

  const { screen } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)

  await expect.element(screen.getByRole('img', { name: 'United Kingdom' })).toBeInTheDocument()
})

test('shows the tags of each contact', async () => {
  worker.use(handleSiteContactsList({ body: page([{ ...ALICE, tags: ['vip', 'beta'] }, BOB], 2) }))

  const { screen } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('vip', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('beta', { exact: true })).toBeInTheDocument()
})

test('shows each contact with an avatar of their initials', async () => {
  worker.use(handleSiteContactsList({ body: page([ALICE, BOB], 2) }))

  const { screen } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('AS', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('BO', { exact: true })).toBeInTheDocument()
})

test('shows the empty state when there are no contacts', async () => {
  worker.use(handleSiteContactsList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Create your first contact.')).toBeInTheDocument()
})

test('shows an error alert when the list fails to load', async () => {
  worker.use(handleSiteContactsList(() => problem(500)))

  const { screen } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Failed to load contacts')).toBeInTheDocument()
})

test('Add contact opens the create form in a modal', async () => {
  worker.use(
    handleSiteContactsList({ body: page([], 0) }),
    handleSiteTagsList({ body: page([], 0) }),
  )

  const { screen } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)
  await screen.getByRole('button', { name: 'Add contact' }).click()

  await expect.element(screen.getByRole('dialog')).toBeVisible()
  await expect.element(screen.getByLabelText('Email')).toBeVisible()
})

test('offers view and edit per contact, and no delete', async () => {
  worker.use(handleSiteContactsList({ body: page([ALICE], 1) }))

  const { screen } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('alice@example.com')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await expect.element(screen.getByRole('menuitem', { name: 'View' })).toBeInTheDocument()
  await expect.element(screen.getByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
  await expect.element(screen.getByRole('menuitem', { name: 'Delete' })).not.toBeInTheDocument()
})

test('the page in the URL is the page of the contacts requested', async () => {
  const pages: (string | null)[] = []
  worker.use(
    handleSiteContactsList(({ request }) => {
      pages.push(new URL(request.url).searchParams.get('page'))
      return HttpResponse.json({ items: [ALICE], totalItems: 25 })
    }),
  )

  await renderWithRouter(<ContactsListPage />, {
    ...LIST_ROUTE,
    initialPath: `${LIST_ROUTE.initialPath}?page=2`,
  })

  await expect.poll(() => pages).toEqual(['2'])
})

test('choosing a page of the contacts puts it in the URL', async () => {
  worker.use(handleSiteContactsList(() => HttpResponse.json({ items: [ALICE], totalItems: 25 })))

  const { screen, navigate } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('alice@example.com')).toBeInTheDocument()
  await screen.getByRole('button', { name: '2', exact: true }).click()

  expect(navigate).toHaveBeenCalledWith(expect.objectContaining({ search: expect.any(Function) }))
})

test('typing in the search field puts the email in the URL search', async () => {
  worker.use(handleSiteContactsList({ body: page([ALICE, BOB], 2) }))

  const { screen, navigate } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)
  // Typing waits for the page to settle: on a busy CI runner a search typed the moment the test
  // router rendered was lost twice, for a reason not found. Typing while the list itself is still
  // loading reaches the URL (the next test), which is what a person can do.
  await expect.element(screen.getByText('bob@example.com')).toBeInTheDocument()
  await screen.getByRole('textbox', { name: 'Search by email' }).fill('bob')

  await expect.poll(() => navigate).toHaveBeenCalledWith(expect.objectContaining({ replace: true }))
  await expect
    .poll(() => searches(navigate, { tag: 'vip', page: 3 }))
    .toContainEqual({ email: 'bob', tag: 'vip', page: undefined })
})

test('a search typed while the list is still loading reaches the URL search', async () => {
  const { promise: answered, resolve: answer } = Promise.withResolvers<undefined>()
  worker.use(
    handleSiteContactsList(async () => {
      await answered
      return HttpResponse.json(page([ALICE, BOB], 2))
    }),
  )

  const { screen, navigate } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)
  await screen.getByRole('textbox', { name: 'Search by email' }).fill('bob')

  await expect.poll(() => searches(navigate, {})).toContainEqual({ email: 'bob', page: undefined })
  answer(undefined)
  await expect.element(screen.getByText('bob@example.com')).toBeInTheDocument()
})

test('clearing the search field takes the email out of the URL search', async () => {
  worker.use(handleSiteContactsList({ body: page([BOB], 1) }))

  const { screen, navigate } = await renderWithRouter(<ContactsListPage />, {
    ...LIST_ROUTE,
    initialPath: `${LIST_ROUTE.initialPath}?email=bob`,
  })
  await expect.element(screen.getByText('bob@example.com')).toBeInTheDocument()
  await screen.getByRole('textbox', { name: 'Search by email' }).fill('   ')

  await expect
    .poll(() => searches(navigate, { email: 'bob' }))
    .toContainEqual({ email: undefined, page: undefined })
})

test('the search field follows the URL search when back or forward changes it', async () => {
  worker.use(handleSiteContactsList({ body: page([ALICE, BOB], 2) }))

  const { screen, router } = await renderWithRouter(<ContactsListPage />, {
    ...LIST_ROUTE,
    initialPath: `${LIST_ROUTE.initialPath}?email=bob`,
  })
  const field = screen.getByRole('textbox', { name: 'Search by email' })
  await expect.element(field).toHaveValue('bob')

  router.history.push(`${LIST_ROUTE.initialPath}?email=alice`)
  await expect.element(field).toHaveValue('alice')

  router.history.push(LIST_ROUTE.initialPath)
  await expect.element(field).toHaveValue('')
})

test('the field keeps what was typed when the URL search is its trimmed text', async () => {
  const requested: (string | null)[] = []
  worker.use(
    handleSiteContactsList(({ request }) => {
      requested.push(new URL(request.url).searchParams.get('email'))
      return HttpResponse.json(page([BOB], 1))
    }),
  )

  const { screen, router } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('bob@example.com')).toBeInTheDocument()
  const field = screen.getByRole('textbox', { name: 'Search by email' })
  await field.fill(' bob ')

  router.history.push(`${LIST_ROUTE.initialPath}?email=bob`)
  // The page has read the new URL search once it asks for that email's contacts.
  await expect.poll(() => requested).toContain('bob')
  await expect.element(field).toHaveValue(' bob ')
})

test('the email in the URL search filters the request and fills the field', async () => {
  const requested: (string | null)[] = []
  worker.use(
    handleSiteContactsList(({ request }) => {
      requested.push(new URL(request.url).searchParams.get('email'))
      return HttpResponse.json(page([BOB], 1))
    }),
  )

  const { screen } = await renderWithRouter(<ContactsListPage />, {
    ...LIST_ROUTE,
    initialPath: `${LIST_ROUTE.initialPath}?email=bob`,
  })

  await expect.element(screen.getByText('bob@example.com')).toBeInTheDocument()
  await expect.element(screen.getByRole('textbox', { name: 'Search by email' })).toHaveValue('bob')
  expect(requested).toEqual(['bob'])
})

test('the tag and the country in the URL search filter the request and fill the filters', async () => {
  const requested: { tag: string | null; country: string | null }[] = []
  worker.use(
    handleSiteContactsList(({ request }) => {
      const params = new URL(request.url).searchParams
      requested.push({ tag: params.get('tag'), country: params.get('country') })
      return HttpResponse.json(page([ALICE], 1))
    }),
    // The Tag of the link is past the Tags the filter fetched: it still shows.
    handleSiteTagsList({
      body: page([{ id: '1', name: 'beta', createdAt: TIMESTAMPS.createdAt }], 1),
    }),
  )

  const { screen } = await renderWithRouter(<ContactsListPage />, {
    ...LIST_ROUTE,
    initialPath: `${LIST_ROUTE.initialPath}?tag=vip&country=GB`,
  })

  await expect.element(screen.getByText('alice@example.com')).toBeInTheDocument()
  expect(requested).toEqual([{ tag: 'vip', country: 'GB' }])
  await expect.element(screen.getByRole('combobox', { name: 'Tag' })).toHaveValue('vip')
  await expect
    .element(screen.getByRole('combobox', { name: 'Country' }))
    .toHaveValue('United Kingdom')
})

test('picking a tag puts it in the URL search and keeps the email search', async () => {
  worker.use(
    handleSiteContactsList({ body: page([ALICE], 1) }),
    handleSiteTagsList({
      body: page([{ id: '1', name: 'vip', createdAt: TIMESTAMPS.createdAt }], 1),
    }),
  )

  const { screen, navigate } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('alice@example.com')).toBeInTheDocument()
  await screen.getByRole('combobox', { name: 'Tag' }).click()
  await screen.getByRole('option', { name: 'vip' }).click()

  await expect
    .poll(() => searches(navigate, { email: 'ali', page: 2 }))
    .toContainEqual({ email: 'ali', tag: 'vip', page: undefined })
})

test('a filter that matches nothing does not invite to create the first contact', async () => {
  worker.use(handleSiteContactsList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<ContactsListPage />, {
    ...LIST_ROUTE,
    initialPath: `${LIST_ROUTE.initialPath}?tag=vip`,
  })

  await expect.element(screen.getByText('No contacts match')).toBeInTheDocument()
  await expect.element(screen.getByText('Create your first contact.')).not.toBeInTheDocument()
})
