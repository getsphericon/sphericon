import { useQuery } from '@tanstack/react-query'
import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { siteTagsListOptions } from '../generated/site/@tanstack/react-query.gen.ts'
import {
  handleSiteTagsList,
  handleSiteWorkspacesList,
  handleSiteWorkspacesSetSecondFactorRequirement,
} from '../generated/site/msw.gen.ts'
import type { SiteWorkspaceResource } from '../generated/site/types.gen.ts'
import { overviewRoute, securityRoute, workspaceRoute } from '../router.tsx'
import { page } from '../test/payloads.ts'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { worker } from '../test/worker.ts'
import { WorkspaceLayout } from './WorkspaceLayout.tsx'

const MOUNT = routeMount(workspaceRoute, { slug: 'acme' })

const workspace = (over: Partial<SiteWorkspaceResource>): SiteWorkspaceResource => ({
  id: '1',
  name: 'Acme',
  slug: 'acme',
  collectKey: 'ck',
  ingestKey: 'ik',
  postalAddress: '',
  accountLinkAvailable: false,
  holds: [],
  role: 'member',
  createdAt: '2026-01-01T00:00:00Z',
  ...over,
})

const list = (items: SiteWorkspaceResource[]) => handleSiteWorkspacesList({ body: items })

test('renders the workspace sidebar and no suspension banner for an active workspace', async () => {
  worker.use(list([workspace({})]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Overview', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Sending is suspended')).not.toBeInTheDocument()
})

test('a suspended workspace shows the banner with the reason', async () => {
  worker.use(list([workspace({ suspendedAt: '2026-02-01T00:00:00Z', suspensionReason: 'spam' })]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Sending is suspended')).toBeInTheDocument()
  await expect.element(screen.getByText(/Reason: spam/)).toBeInTheDocument()
})

test('a suspended workspace tells the owner where to appeal', async () => {
  worker.use(
    list([
      workspace({ suspendedAt: '2026-02-01T00:00:00Z', supportEmail: 'support@sphericon.test' }),
    ]),
  )
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect
    .element(screen.getByText(/To appeal, email support@sphericon.test/))
    .toBeInTheDocument()
})

test('without a support contact the owner is sent to the administrator', async () => {
  worker.use(list([workspace({ suspendedAt: '2026-02-01T00:00:00Z' })]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect
    .element(screen.getByText(/To appeal, contact your sphericon administrator/))
    .toBeInTheDocument()
})

test('a suspension without a reason omits the reason line', async () => {
  worker.use(list([workspace({ suspendedAt: '2026-02-01T00:00:00Z' })]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Sending is suspended')).toBeInTheDocument()
  await expect.element(screen.getByText(/Reason:/)).not.toBeInTheDocument()
})

const HOLD = {
  key: 'k1',
  reasonCode: 'review',
  message: 'Please wait a day.',
  since: '2026-02-01T00:00:00Z',
  billing: false,
}

const BILLING_HOLD = {
  key: 'bill',
  reasonCode: 'non_payment',
  message: 'Your invoice is overdue.',
  since: '2026-02-01T00:00:00Z',
  billing: true,
}

test('a held workspace shows the hold banner with the setter message as given, to a member', async () => {
  worker.use(list([workspace({ role: 'member', holds: [HOLD] })]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Sending is on hold')).toBeInTheDocument()
  await expect.element(screen.getByText('Please wait a day.')).toBeInTheDocument()
  await expect.element(screen.getByText('Sending is suspended')).not.toBeInTheDocument()
})

test('every active hold message is shown', async () => {
  const other = { ...HOLD, key: 'k2', message: 'Second message.' }
  worker.use(list([workspace({ holds: [HOLD, other] })]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Please wait a day.')).toBeInTheDocument()
  await expect.element(screen.getByText('Second message.')).toBeInTheDocument()
})

test('a suspended workspace shows the suspension banner and no hold banner', async () => {
  worker.use(list([workspace({ suspendedAt: '2026-02-01T00:00:00Z' })]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Sending is suspended')).toBeInTheDocument()
  await expect.element(screen.getByText('Sending is on hold')).not.toBeInTheDocument()
})

test('a workspace held and suspended shows both banners', async () => {
  worker.use(list([workspace({ suspendedAt: '2026-02-01T00:00:00Z', holds: [HOLD] })]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Sending is suspended')).toBeInTheDocument()
  await expect.element(screen.getByText('Sending is on hold')).toBeInTheDocument()
})

test('a billing hold shows its own banner, not the generic hold or the suspension one', async () => {
  worker.use(list([workspace({ role: 'member', holds: [BILLING_HOLD] })]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Sending is paused for billing')).toBeInTheDocument()
  await expect.element(screen.getByText('Your invoice is overdue.')).toBeInTheDocument()
  await expect.element(screen.getByText('Sending is on hold')).not.toBeInTheDocument()
  await expect.element(screen.getByText('Sending is suspended')).not.toBeInTheDocument()
})

test('a billing hold beside another hold and a suspension shows all three banners', async () => {
  worker.use(
    list([workspace({ suspendedAt: '2026-02-01T00:00:00Z', holds: [HOLD, BILLING_HOLD] })]),
  )
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Sending is suspended')).toBeInTheDocument()
  await expect.element(screen.getByText('Sending is on hold')).toBeInTheDocument()
  await expect.element(screen.getByText('Sending is paused for billing')).toBeInTheDocument()
  await expect.element(screen.getByText('Please wait a day.')).toBeInTheDocument()
  await expect.element(screen.getByText('Your invoice is overdue.')).toBeInTheDocument()
})

const BILLING_URL = 'https://billing.sphericon.test/acme'

test.each(['owner', 'admin'] as const)(
  'a held workspace links the %s to the configured billing site',
  async (role) => {
    worker.use(list([workspace({ role, holds: [HOLD], billingUrl: BILLING_URL })]))
    const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

    await expect
      .element(screen.getByRole('link', { name: 'Go to billing' }))
      .toHaveAttribute('href', BILLING_URL)
    await expect.element(screen.getByText(/contact an Owner or Admin/)).not.toBeInTheDocument()
  },
)

test('a held workspace tells a plain member to contact an Owner or Admin, without a link', async () => {
  worker.use(list([workspace({ role: 'member', holds: [HOLD], billingUrl: BILLING_URL })]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText(/contact an Owner or Admin/)).toBeInTheDocument()
  await expect.element(screen.getByRole('link', { name: 'Go to billing' })).not.toBeInTheDocument()
})

test('with no billing site configured a hold shows no billing link and no billing text', async () => {
  worker.use(list([workspace({ role: 'owner', holds: [HOLD] })]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Sending is on hold')).toBeInTheDocument()
  await expect.element(screen.getByRole('link', { name: 'Go to billing' })).not.toBeInTheDocument()
  await expect.element(screen.getByText(/contact an Owner or Admin/)).not.toBeInTheDocument()
})

test('the billing link rides only the hold banner, not the suspension notice or a free workspace', async () => {
  worker.use(
    list([
      workspace({ role: 'owner', suspendedAt: '2026-02-01T00:00:00Z', billingUrl: BILLING_URL }),
    ]),
  )
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Sending is suspended')).toBeInTheDocument()
  await expect.element(screen.getByRole('link', { name: 'Go to billing' })).not.toBeInTheDocument()
})

test('the billing banner links an owner to the billing site', async () => {
  worker.use(
    list([
      workspace({ role: 'owner', holds: [BILLING_HOLD], billingUrl: BILLING_URL }),
      workspace({
        id: '2',
        slug: 'beta',
        role: 'member',
        holds: [BILLING_HOLD],
        billingUrl: BILLING_URL,
      }),
    ]),
  )
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect
    .element(screen.getByRole('link', { name: 'Go to billing' }))
    .toHaveAttribute('href', BILLING_URL)
})

test('an unaffected workspace shows no hold banner', async () => {
  worker.use(list([workspace({})]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Overview', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Sending is on hold')).not.toBeInTheDocument()
})

test('the switcher navigates to the chosen workspace overview', async () => {
  worker.use(list([workspace({}), workspace({ id: '2', name: 'Beta', slug: 'beta' })]))
  const { screen, navigate } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await screen.getByRole('combobox').click()
  await screen.getByRole('option', { name: 'Beta' }).click()

  expect(navigate).toHaveBeenCalledWith({ to: overviewRoute.to, params: { slug: 'beta' } })
})

// TagsProbe stands in for a workspace page whose query the server refuses.
function TagsProbe() {
  useQuery({ ...siteTagsListOptions({ path: { slug: 'acme' } }), retry: false })
  return null
}

const DAY = 24 * 60 * 60 * 1000

test('during the grace of a Two-factor requirement a banner leads to enrollment', async () => {
  const endsAt = new Date(Date.now() + 3 * DAY).toISOString()
  worker.use(list([workspace({ secondFactorGraceEndsAt: endsAt })]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect
    .element(screen.getByText('This workspace requires two-factor authentication'))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Set up two-factor authentication' }))
    .toHaveAttribute('href', securityRoute.to)
  await expect
    .element(screen.getByText('Two-factor authentication required'))
    .not.toBeInTheDocument()
})

test('after the grace the workspace is replaced by the screen that leads to enrollment', async () => {
  const endsAt = new Date(Date.now() - DAY).toISOString()
  worker.use(list([workspace({ secondFactorGraceEndsAt: endsAt })]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Two-factor authentication required')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Set up two-factor authentication' }))
    .toHaveAttribute('href', securityRoute.to)
})

test('a 403 second_factor_required from the workspace shows the blocked screen', async () => {
  worker.use(
    list([workspace({})]),
    handleSiteTagsList(() => problem(403, { title: 'Forbidden', code: 'second_factor_required' })),
  )
  const { screen } = await renderWithRouter(
    <>
      <WorkspaceLayout />
      <TagsProbe />
    </>,
    MOUNT,
  )

  await expect.element(screen.getByText('Two-factor authentication required')).toBeInTheDocument()
})

// Story 35: an Owner or Admin withheld by the requirement can still lift it.
test('a withheld owner can turn the requirement off from the blocked screen', async () => {
  const bodies: string[] = []
  let current = workspace({
    role: 'owner',
    secondFactorRequiredAt: '2026-03-01T00:00:00Z',
    secondFactorGraceEndsAt: new Date(Date.now() - DAY).toISOString(),
  })
  worker.use(
    handleSiteWorkspacesList(() => HttpResponse.json([current])),
    handleSiteWorkspacesSetSecondFactorRequirement(async ({ request }) => {
      bodies.push(await request.text())
      current = workspace({ role: 'owner' })
      return HttpResponse.json(current)
    }),
    handleSiteTagsList(() =>
      bodies.length > 0
        ? HttpResponse.json(page([]))
        : problem(403, { title: 'Forbidden', code: 'second_factor_required' }),
    ),
  )
  const { screen } = await renderWithRouter(
    <>
      <WorkspaceLayout />
      <TagsProbe />
    </>,
    MOUNT,
  )

  await expect.element(screen.getByText('Two-factor authentication required')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Turn off the requirement' }).click()

  await expect.poll(() => bodies).toEqual(['{"required":false}'])
  await expect
    .element(screen.getByText('Two-factor authentication required'))
    .not.toBeInTheDocument()
})

test('a withheld member is not offered to turn the requirement off', async () => {
  worker.use(
    list([
      workspace({
        secondFactorRequiredAt: '2026-03-01T00:00:00Z',
        secondFactorGraceEndsAt: new Date(Date.now() - DAY).toISOString(),
      }),
    ]),
  )
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Two-factor authentication required')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Turn off the requirement' }).elements()).toHaveLength(
    0,
  )
})
