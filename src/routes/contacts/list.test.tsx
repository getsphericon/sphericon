import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteContactsDelete, handleSiteContactsList } from '../../generated/site/msw.gen.ts'
import { contactsRoute } from '../../router.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ContactsListPage } from './list.tsx'

const LIST_ROUTE = routeMount(contactsRoute, { slug: 'test' })

const ALICE = {
  id: '1',
  email: 'alice@example.com',
  firstName: 'Alice',
  lastName: 'Smith',
  ...TIMESTAMPS,
}
const BOB = { id: '2', email: 'bob@example.com', ...TIMESTAMPS }

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

test('Add contact links to the create page', async () => {
  worker.use(handleSiteContactsList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)

  await expect
    .element(screen.getByRole('link', { name: 'Add contact' }))
    .toHaveAttribute('href', '/workspaces/test/contacts/new')
})

test('deleting a contact asks for confirmation, deletes it and refreshes the list', async () => {
  const deleted: string[] = []
  let listFetches = 0
  worker.use(
    handleSiteContactsList(() => {
      listFetches++
      return HttpResponse.json({ items: deleted.length ? [BOB] : [ALICE, BOB], totalItems: 2 })
    }),
    handleSiteContactsDelete(({ params }) => {
      deleted.push(params.id)
      return new HttpResponse(null, { status: 204 })
    }),
  )

  const { screen } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('alice@example.com')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'Delete' }).first().click()
  expect(deleted).toEqual([])
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.poll(() => deleted).toEqual(['1'])
  await expect.element(screen.getByText('Contact deleted')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.element(screen.getByText('alice@example.com')).not.toBeInTheDocument()
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

test('a failed delete shows the error toast and keeps the contact', async () => {
  worker.use(
    handleSiteContactsList({ body: page([ALICE], 1) }),
    handleSiteContactsDelete(() => problem(500)),
  )

  const { screen } = await renderWithRouter(<ContactsListPage />, LIST_ROUTE)
  await screen.getByRole('button', { name: 'Delete' }).first().click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.element(screen.getByText('Failed to delete', { exact: false })).toBeInTheDocument()
  await expect.element(screen.getByText('alice@example.com')).toBeInTheDocument()
})
