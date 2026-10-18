import { useQuery } from '@tanstack/react-query'
import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  siteContactsGetOptions,
  siteContactsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import {
  handleSiteContactsCreate,
  handleSiteContactsGet,
  handleSiteContactsList,
  handleSiteContactsUpdate,
} from '../../generated/site/msw.gen.ts'
import { Route as contactsEditRoute } from '../../routes/workspaces/$slug/contacts/$contactId/edit.tsx'
import { Route as contactsCreateRoute } from '../../routes/workspaces/$slug/contacts/new.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ContactCreatePage, ContactEditPage } from './resource.tsx'

const CREATE_ROUTE = routeMount(contactsCreateRoute, { slug: 'test' })

// Stands in for the contacts list screen: an active list query, so invalidation after a save
// is observable as a refetch.
function ListProbe() {
  useQuery(siteContactsListOptions({ path: { slug: 'test' } }))
  return null
}

test('creating a contact sends only the filled keys, refreshes the list and opens the edit page', async () => {
  const bodies: unknown[] = []
  let listFetches = 0
  worker.use(
    handleSiteContactsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(
        { id: '42', email: 'ada@example.com', firstName: 'Ada' },
        { status: 201 },
      )
    }),
    handleSiteContactsList(() => {
      listFetches++
      return HttpResponse.json({ items: [], totalItems: 0 })
    }),
  )

  const { screen, navigate } = await renderWithRouter(
    <>
      <ListProbe />
      <ContactCreatePage />
    </>,
    CREATE_ROUTE,
  )
  await expect.poll(() => listFetches).toBe(1)

  await screen.getByLabelText('Email').fill('ada@example.com')
  await screen.getByLabelText('First name').fill('Ada')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([{ email: 'ada@example.com', firstName: 'Ada' }])
  await expect.element(screen.getByText('Contact created')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.poll(() => navigate.mock.calls.length).toBe(1)
  expect(navigate.mock.calls[0]?.[0]).toMatchObject({
    params: { slug: 'test', contactId: '42' },
  })
})

test('a malformed email is rejected before anything is sent', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteContactsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ id: '1' }, { status: 201 })
    }),
  )

  const { screen } = await renderWithRouter(<ContactCreatePage />, CREATE_ROUTE)

  await screen.getByLabelText('Email').fill('not-an-email')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('Invalid email address')).toBeVisible()
  expect(bodies).toEqual([])
})

const EDIT_ROUTE = routeMount(contactsEditRoute, { slug: 'test', contactId: '7' })

const LOADED_CONTACT = {
  id: '7',
  email: 'ada@example.com',
  phone: null,
  firstName: 'Ada',
  customFields: { plan: 'pro' },
}

// Active detail query next to the edit page, so its invalidation is observable.
function DetailProbe() {
  useQuery(siteContactsGetOptions({ path: { slug: 'test', id: '7' } }))
  return null
}

test('editing shows existing values with nulls as blanks; a cleared field is sent as null', async () => {
  const bodies: unknown[] = []
  let detailFetches = 0
  let listFetches = 0
  worker.use(
    handleSiteContactsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ id: '7', email: 'ada@example.com' })
    }),
    handleSiteContactsList(() => {
      listFetches++
      return HttpResponse.json({ items: [], totalItems: 0 })
    }),
    handleSiteContactsGet(() => {
      detailFetches++
      return HttpResponse.json(LOADED_CONTACT)
    }),
  )

  const { screen } = await renderWithRouter(
    <>
      <ListProbe />
      <DetailProbe />
      <ContactEditPage />
    </>,
    EDIT_ROUTE,
  )

  await expect.element(screen.getByLabelText('Email')).toHaveValue('ada@example.com')
  await expect.element(screen.getByLabelText('First name')).toHaveValue('Ada')
  await expect.element(screen.getByLabelText('Phone')).toHaveValue('')

  await screen.getByLabelText('First name').fill('')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        subjectId: null,
        email: 'ada@example.com',
        phone: null,
        firstName: null,
        lastName: null,
        timeZone: null,
      },
    ])
  await expect.element(screen.getByText('Contact updated')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.poll(() => detailFetches).toBeGreaterThan(1)
})

test('editing shows an error alert when the contact cannot be loaded', async () => {
  worker.use(
    handleSiteContactsGet(() => problem(404, { title: 'Not Found', detail: 'contact not found' })),
  )

  const { screen } = await renderWithRouter(<ContactEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByText('Failed to load contacts')).toBeVisible()
  await expect.element(screen.getByText('contact not found')).toBeVisible()
})

test('editing shows no form while the contact loads, then the loaded values', async () => {
  const gate = Promise.withResolvers<void>()
  worker.use(
    handleSiteContactsGet(async () => {
      await gate.promise
      return HttpResponse.json(LOADED_CONTACT)
    }),
  )

  const { screen } = await renderWithRouter(<ContactEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByLabelText('Email')).not.toBeInTheDocument()
  gate.resolve()

  await expect.element(screen.getByLabelText('Email')).toHaveValue('ada@example.com')
})
