import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'

import { handleSiteContactsList, handleSiteTagsList } from '../../generated/site/msw.gen.ts'
import { Route as contactsRoute } from '../../routes/workspaces/$workspaceId/contacts/index.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ContactsListPage } from './list.tsx'

const LIST_ROUTE = routeMount(contactsRoute, { workspaceId: 'TestWs01' })

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
  await expect.element(screen.getByText('Smith')).toBeInTheDocument()
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

test('changing the page requests page 2 of the contacts', async () => {
  const pages: (string | null)[] = []
  worker.use(
    handleSiteContactsList(({ request }) => {
      pages.push(new URL(request.url).searchParams.get('page'))
      return HttpResponse.json({ items: [ALICE], totalItems: 25 })
    }),
  )

  const { screen } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('alice@example.com')).toBeInTheDocument()
  await screen.getByRole('button', { name: '2', exact: true }).click()

  await expect.poll(() => pages).toEqual(['1', '2'])
})

test('typing in the search field puts the email in the URL search', async () => {
  worker.use(handleSiteContactsList({ body: page([ALICE, BOB], 2) }))

  const { screen, navigate } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)
  await screen.getByRole('textbox', { name: 'Search by email' }).fill('bob')

  await vi.waitFor(() =>
    expect(navigate).toHaveBeenCalledWith(
      expect.objectContaining({ search: { email: 'bob' }, replace: true }),
    ),
  )
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
