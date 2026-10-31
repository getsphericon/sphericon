import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleStaffSignupItemsCreate,
  handleStaffSignupItemsGet,
  handleStaffSignupItemsList,
  handleStaffSignupItemsListVersions,
  handleStaffSignupItemsPublish,
  handleStaffSignupItemsReorder,
  handleStaffSignupItemsArchive,
  handleStaffSignupItemsSaveDraft,
  handleStaffSignupItemsUpdate,
} from '../generated/staff/msw.gen.ts'
import { SignupItemKind } from '../generated/staff/types.gen.ts'
import type {
  StaffLegalDocumentVersion,
  StaffSignupItemInput,
  StaffSignupItemResource,
  StaffSignupItemUpdate,
} from '../generated/staff/types.gen.ts'
import { Route as consoleSignupItemRoute } from '../routes/console/_authed/signup-items.$itemId.tsx'
import { Route as consoleSignupItemsRoute } from '../routes/console/_authed/signup-items.index.tsx'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { worker } from '../test/worker.ts'
import { ConsoleSignupItemPage } from './signupItem.tsx'
import { ConsoleSignupItemsPage } from './signupItems.tsx'

const LIST = routeMount(consoleSignupItemsRoute)
const ITEM = routeMount(consoleSignupItemRoute, { itemId: '7' })

function item(over: Partial<StaffSignupItemResource> = {}): StaffSignupItemResource {
  return {
    id: '7',
    key: 'terms',
    title: 'Terms of service',
    kind: SignupItemKind.REQUIRED_ACCEPTANCE,
    position: 1,
    mailingsConsent: false,
    createdAt: '2026-10-01T00:00:00Z',
    ...over,
  }
}

function version(label: string, email = 'staff@sphericon.test'): StaffLegalDocumentVersion {
  return { label, publishedAt: '2026-10-01T00:00:00Z', publishedBy: { id: '1', email } }
}

// The item page reads the item and its published versions.
function serveItem(body: StaffSignupItemResource, versions: StaffLegalDocumentVersion[] = []) {
  worker.use(
    handleStaffSignupItemsGet({ body }),
    handleStaffSignupItemsListVersions({ body: { items: versions } }),
  )
}

test('lists the items with their current version, or that none is published', async () => {
  worker.use(
    handleStaffSignupItemsList({
      body: {
        items: [
          item({ currentVersion: version('2026-10') }),
          item({
            id: '8',
            key: 'news',
            title: 'News',
            kind: SignupItemKind.OPTIONAL_CONSENT,
            mailingsConsent: true,
          }),
        ],
      },
    }),
  )

  const { screen } = await renderWithRouter(<ConsoleSignupItemsPage />, LIST)

  await expect.element(screen.getByText('Terms of service', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('2026-10', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Not published', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Mailings consent', { exact: true })).toBeInTheDocument()
})

test('an archived item is marked as archived in the list', async () => {
  worker.use(
    handleStaffSignupItemsList({
      body: { items: [item({ archivedAt: '2026-10-20T00:00:00Z' })] },
    }),
  )

  const { screen } = await renderWithRouter(<ConsoleSignupItemsPage />, LIST)

  await expect.element(screen.getByText('Archived', { exact: true })).toBeInTheDocument()
})

test('moving an item sends the new order of every item', async () => {
  const orders: string[][] = []
  const items = [
    item(),
    item({ id: '8', key: 'privacy', title: 'Privacy notice', position: 2 }),
    item({ id: '9', key: 'news', title: 'News', position: 3 }),
  ]
  worker.use(
    handleStaffSignupItemsList({ body: { items } }),
    handleStaffSignupItemsReorder(async ({ request }) => {
      const { ids } = await request.json()
      orders.push(ids)
      return HttpResponse.json({ items })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<ConsoleSignupItemsPage />, LIST)

  await screen.getByRole('button', { name: 'Move Terms of service down' }).click()

  await expect.poll(() => orders).toEqual([['8', '7', '9']])
  expect(navigate).not.toHaveBeenCalled()
  await expect
    .element(screen.getByRole('button', { name: 'Move Terms of service up' }))
    .toBeDisabled()
  await expect.element(screen.getByRole('button', { name: 'Move News down' })).toBeDisabled()
})

test('adding an item sends it and opens it', async () => {
  const sent: StaffSignupItemInput[] = []
  worker.use(
    handleStaffSignupItemsList({ body: { items: [] } }),
    handleStaffSignupItemsCreate(async ({ request }) => {
      sent.push(await request.json())
      return HttpResponse.json(
        item({ id: '9', key: 'news', title: 'News', kind: SignupItemKind.OPTIONAL_CONSENT }),
        {
          status: 201,
        },
      )
    }),
  )
  const { screen, navigate } = await renderWithRouter(<ConsoleSignupItemsPage />, LIST)

  await screen.getByLabelText('Key').fill('news')
  await screen.getByLabelText('Title').fill('News')
  await screen.getByRole('combobox', { name: 'Kind' }).click()
  await screen.getByRole('option', { name: 'Optional consent' }).click()
  await screen.getByLabelText('Consent to the mailings').click()
  await screen.getByRole('button', { name: 'Add item' }).click()

  await expect
    .poll(() => sent)
    .toEqual([
      { key: 'news', title: 'News', kind: SignupItemKind.OPTIONAL_CONSENT, mailingsConsent: true },
    ])
  await expect
    .poll(() => navigate.mock.calls[0]?.[0])
    .toMatchObject({ to: consoleSignupItemRoute.to, params: { itemId: '9' } })
})

test('an item is checked against the contract before it is sent', async () => {
  let sent = 0
  worker.use(
    handleStaffSignupItemsList({ body: { items: [] } }),
    handleStaffSignupItemsCreate(() => {
      sent++
      return HttpResponse.json(item(), { status: 201 })
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleSignupItemsPage />, LIST)

  await screen.getByLabelText('Title').fill('   ')
  await screen.getByRole('button', { name: 'Add item' }).click()

  await expect.poll(() => screen.getByText('This field is required').elements().length).toBe(3)
  await screen.getByLabelText('Key').fill('Not A Key')
  await screen.getByRole('button', { name: 'Add item' }).click()
  await expect.element(screen.getByText(/match pattern/)).toBeInTheDocument()
  expect(sent).toBe(0)
})

test('saving the draft sends its Markdown, and the preview renders it', async () => {
  const drafts: string[] = []
  serveItem(item())
  worker.use(
    handleStaffSignupItemsSaveDraft(async ({ request }) => {
      const { markdown } = await request.json()
      drafts.push(markdown)
      return HttpResponse.json(item({ draft: markdown }))
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleSignupItemPage />, ITEM)

  await screen.getByLabelText('Draft').fill('## Using the service')
  await screen.getByRole('button', { name: 'Save draft' }).click()
  await screen.getByRole('tab', { name: 'Preview' }).click()

  await expect.poll(() => drafts).toEqual(['## Using the service'])
  await expect
    .element(screen.getByRole('heading', { name: 'Using the service' }))
    .toBeInTheDocument()
})

// A new version of an optional consent or a notice stops no one (ADR 0062): it is published
// without a warning, and the history shows it.
test.each<SignupItemKind>([SignupItemKind.OPTIONAL_CONSENT, SignupItemKind.NOTICE])(
  'publishing a version of an item of kind %s sends the label at once and lists it',
  async (kind) => {
    const labels: string[] = []
    let stored = item({ kind, draft: '# Terms' })
    let versions: StaffLegalDocumentVersion[] = []
    worker.use(
      handleStaffSignupItemsGet(() => HttpResponse.json(stored)),
      handleStaffSignupItemsListVersions(() => HttpResponse.json({ items: versions })),
      handleStaffSignupItemsPublish(async ({ request }) => {
        const { label } = await request.json()
        labels.push(label)
        stored = { ...stored, currentVersion: version(label) }
        versions = [version(label)]
        return HttpResponse.json(stored, { status: 201 })
      }),
    )
    const { screen } = await renderWithRouter(<ConsoleSignupItemPage />, ITEM)

    await screen.getByLabelText('Version label').fill('2026-10')
    await screen.getByRole('button', { name: 'Publish' }).click()

    await expect.poll(() => labels).toEqual(['2026-10'])
    await expect
      .element(screen.getByText('Current version: 2026-10', { exact: false }))
      .toBeInTheDocument()
    await expect
      .element(screen.getByRole('link', { name: 'Open the public page' }))
      .toHaveAttribute('href', '/legal/terms')
    await expect.element(screen.getByRole('cell', { name: '2026-10' })).toBeInTheDocument()
  },
)

test('publishing a required acceptance warns that it stops every User who has not accepted it', async () => {
  const labels: string[] = []
  serveItem(item({ draft: '# Terms', currentVersion: version('v1') }), [version('v1')])
  worker.use(
    handleStaffSignupItemsPublish(async ({ request }) => {
      const { label } = await request.json()
      labels.push(label)
      return HttpResponse.json(item({ currentVersion: version(label) }), { status: 201 })
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleSignupItemPage />, ITEM)

  await screen.getByLabelText('Version label').fill('v2')
  await screen.getByRole('button', { name: 'Publish' }).click()

  const dialog = screen.getByRole('dialog')
  await expect
    .element(dialog.getByText('every User who has not accepted it', { exact: false }))
    .toBeInTheDocument()
  expect(labels).toEqual([])
  await dialog.getByRole('button', { name: 'Publish' }).click()
  await expect.poll(() => labels).toEqual(['v2'])
})

test('a warning cancelled publishes nothing', async () => {
  const labels: string[] = []
  serveItem(item({ draft: '# Terms' }))
  worker.use(
    handleStaffSignupItemsPublish(async ({ request }) => {
      const { label } = await request.json()
      labels.push(label)
      return HttpResponse.json(item({ currentVersion: version(label) }), { status: 201 })
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleSignupItemPage />, ITEM)

  await screen.getByLabelText('Version label').fill('v1')
  await screen.getByRole('button', { name: 'Publish' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click()

  await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
  expect(labels).toEqual([])
})

test('a draft with unsaved edits cannot be published until it is saved', async () => {
  serveItem(item({ draft: '# Terms' }))
  const { screen } = await renderWithRouter(<ConsoleSignupItemPage />, ITEM)
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeEnabled()

  await screen.getByLabelText('Draft').fill('# Terms, edited')

  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
  await expect
    .element(screen.getByText('Save the draft first: publishing takes the saved text.'))
    .toBeInTheDocument()
})

test('a label the item already has is refused with the reason', async () => {
  serveItem(item({ kind: SignupItemKind.NOTICE, draft: '# Terms' }))
  worker.use(
    handleStaffSignupItemsPublish(() =>
      problem(422, {
        detail: 'this item already has a version with this label',
        errors: { label: ['this item already has a version with this label'] },
      }),
    ),
  )
  const { screen } = await renderWithRouter(<ConsoleSignupItemPage />, ITEM)

  await screen.getByLabelText('Version label').fill('v1')
  await screen.getByRole('button', { name: 'Publish' }).click()

  await expect
    .element(screen.getByText('this item already has a version with this label'))
    .toBeInTheDocument()
})

test('lists every published version with its label, publisher and time', async () => {
  serveItem(item({ currentVersion: version('v2') }), [
    version('v2', 'ada@sphericon.test'),
    version('v1', 'staff@sphericon.test'),
  ])

  const { screen } = await renderWithRouter(<ConsoleSignupItemPage />, ITEM)

  await expect.element(screen.getByRole('cell', { name: 'v2' })).toBeInTheDocument()
  await expect.element(screen.getByRole('cell', { name: 'ada@sphericon.test' })).toBeInTheDocument()
  await expect.element(screen.getByRole('cell', { name: 'v1' })).toBeInTheDocument()
  await expect
    .element(screen.getByRole('cell', { name: 'staff@sphericon.test' }))
    .toBeInTheDocument()
})

test('retitling sends the new title', async () => {
  const updates: StaffSignupItemUpdate[] = []
  serveItem(item())
  worker.use(
    handleStaffSignupItemsUpdate(async ({ request }) => {
      const body = await request.json()
      updates.push(body)
      return HttpResponse.json(item({ title: body.title ?? 'Terms of service' }))
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleSignupItemPage />, ITEM)

  await screen.getByLabelText('Title').fill('Terms of use')
  await screen.getByRole('button', { name: 'Save title' }).click()

  await expect.poll(() => updates).toEqual([{ title: 'Terms of use' }])
})

test('archiving asks first, then shows the item archived', async () => {
  let archived = 0
  let stored = item()
  worker.use(
    handleStaffSignupItemsGet(() => HttpResponse.json(stored)),
    handleStaffSignupItemsListVersions({ body: { items: [] } }),
    handleStaffSignupItemsArchive(() => {
      archived += 1
      stored = { ...stored, archivedAt: '2026-10-29T00:00:00Z' }
      return HttpResponse.json(stored)
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleSignupItemPage />, ITEM)

  await screen.getByRole('button', { name: 'Archive' }).click()
  const dialog = screen.getByRole('dialog')
  await expect
    .element(dialog.getByText('leaves the sign-up form and its public page', { exact: false }))
    .toBeInTheDocument()
  expect(archived).toBe(0)
  await dialog.getByRole('button', { name: 'Archive' }).click()

  await expect.poll(() => archived).toBe(1)
  await expect.element(screen.getByText('Archived', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Archive' })).not.toBeInTheDocument()
})

test('an optional consent is marked as the mailings consent', async () => {
  const marks: (boolean | undefined)[] = []
  serveItem(item({ kind: SignupItemKind.OPTIONAL_CONSENT, key: 'news' }))
  worker.use(
    handleStaffSignupItemsUpdate(async ({ request }) => {
      const { mailingsConsent } = await request.json()
      marks.push(mailingsConsent)
      return HttpResponse.json(
        item({
          kind: SignupItemKind.OPTIONAL_CONSENT,
          key: 'news',
          mailingsConsent: mailingsConsent ?? false,
        }),
      )
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleSignupItemPage />, ITEM)

  await screen.getByRole('switch', { name: 'Consent to the mailings' }).click()

  await expect.poll(() => marks).toEqual([true])
})
