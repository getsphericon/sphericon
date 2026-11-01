import { useQuery } from '@tanstack/react-query'
import { HttpResponse } from 'msw'
import { beforeEach, expect, test, vi } from 'vitest'

import {
  siteContactsGetOptions,
  siteContactsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import {
  handleSiteContactsCreate,
  handleSiteContactsGet,
  handleSiteContactsList,
  handleSiteContactsUpdate,
  handleSiteTagsList,
} from '../../generated/site/msw.gen.ts'
import { Route as contactsEditRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/edit.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$workspaceId/contacts/index.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ContactCreateModal, ContactEditPage } from './resource.tsx'

// The form's tags field suggests the workspace's existing tags.
beforeEach(() => {
  worker.use(handleSiteTagsList({ body: page([{ id: '1', name: 'vip', ...TIMESTAMPS }], 1) }))
})

const CREATE_ROUTE = routeMount(contactsRoute, { workspaceId: 'TestWs01' })

// Stands in for the contacts list screen: an active list query, so invalidation after a save
// is observable as a refetch.
function ListProbe() {
  useQuery(siteContactsListOptions({ path: { workspaceId: 'TestWs01' } }))
  return null
}

test('creating a contact sends the filled keys and its tags, refreshes the list and closes the modal', async () => {
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

  const onClose = vi.fn<() => void>()
  const { screen } = await renderWithRouter(
    <>
      <ListProbe />
      <ContactCreateModal opened onClose={onClose} />
    </>,
    CREATE_ROUTE,
  )
  await expect.poll(() => listFetches).toBe(1)

  await screen.getByLabelText('Email').fill('ada@example.com')
  await screen.getByLabelText('First name').fill('Ada')
  await screen.getByRole('combobox', { name: 'Tags' }).fill('lead')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([{ email: 'ada@example.com', firstName: 'Ada', tags: ['lead'] }])
  await expect.element(screen.getByText('Contact created')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.poll(() => onClose.mock.calls.length).toBe(1)
})

test('picking a country sends its code', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteContactsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ id: '42', email: 'ada@example.com' }, { status: 201 })
    }),
  )

  const { screen } = await renderWithRouter(
    <ContactCreateModal opened onClose={vi.fn<() => void>()} />,
    CREATE_ROUTE,
  )

  await screen.getByLabelText('Email').fill('ada@example.com')
  await screen.getByRole('combobox', { name: 'Country' }).fill('Germ')
  await screen.getByRole('option', { name: 'Germany' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([{ email: 'ada@example.com', country: 'DE', tags: [] }])
})

test('a malformed email is rejected before anything is sent', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteContactsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ id: '1' }, { status: 201 })
    }),
  )

  const { screen } = await renderWithRouter(
    <ContactCreateModal opened onClose={vi.fn<() => void>()} />,
    CREATE_ROUTE,
  )

  await screen.getByLabelText('Email').fill('not-an-email')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('Invalid email address')).toBeVisible()
  expect(bodies).toEqual([])
})

const EDIT_ROUTE = routeMount(contactsEditRoute, { workspaceId: 'TestWs01', contactId: '7' })

const LOADED_CONTACT = {
  id: '7',
  email: 'ada@example.com',
  phone: null,
  firstName: 'Ada',
  customFields: { plan: 'pro' },
  tags: ['vip'],
}

// Active detail query next to the edit page, so its invalidation is observable.
function DetailProbe() {
  useQuery(siteContactsGetOptions({ path: { workspaceId: 'TestWs01', id: '7' } }))
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
        email: 'ada@example.com',
        phone: null,
        firstName: null,
        lastName: null,
        timeZone: null,
        country: null,
        tags: ['vip'],
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
  const gate = Promise.withResolvers<undefined>()
  worker.use(
    handleSiteContactsGet(async () => {
      await gate.promise
      return HttpResponse.json(LOADED_CONTACT)
    }),
  )

  const { screen } = await renderWithRouter(<ContactEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByLabelText('Email')).not.toBeInTheDocument()
  gate.resolve(undefined)

  await expect.element(screen.getByLabelText('Email')).toHaveValue('ada@example.com')
})
