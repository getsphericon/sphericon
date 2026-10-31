import { useQuery } from '@tanstack/react-query'
import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import { siteTagsListOptions } from '../generated/site/@tanstack/react-query.gen.ts'
import {
  handleSiteInstanceGetRelease,
  handleSiteTagsList,
  handleSiteWorkspacesList,
  handleSiteWorkspacesLiftSecondFactorRequirement,
} from '../generated/site/msw.gen.ts'
import type { SiteWorkspaceResource } from '../generated/site/types.gen.ts'
import { Route as securityRoute } from '../routes/account/security.tsx'
import { Route as overviewRoute } from '../routes/workspaces/$workspaceId/index.tsx'
import { Route as workspaceRoute } from '../routes/workspaces/$workspaceId/route.tsx'
import { Route as teamRoute } from '../routes/workspaces/$workspaceId/settings/team.tsx'
import { Route as newWorkspaceRoute } from '../routes/workspaces/new.tsx'
import { page } from '../test/payloads.ts'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { worker } from '../test/worker.ts'
import { WorkspaceLayout } from './WorkspaceLayout.tsx'

const MOUNT = routeMount(workspaceRoute, { workspaceId: 'AcmeWs01' })

// Most users own no Workspace: the release banner is refused to them.
beforeEach(() => {
  worker.use(handleSiteInstanceGetRelease(() => problem(403)))
})

const workspace = (over: Partial<SiteWorkspaceResource>): SiteWorkspaceResource => ({
  id: 'AcmeWs01',
  name: 'Acme',

  collectKey: 'ck',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions: [],
  mcpUrl: 'https://sphericon.test',
  trackerUrl: 'https://t.sphericon.test',
  holds: [],
  sendCap: null,
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

test('the last switcher option opens the create screen', async () => {
  worker.use(list([workspace({})]))
  const { screen, navigate } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await screen.getByRole('combobox').click()
  await screen.getByRole('option', { name: 'Create workspace' }).click()

  expect(navigate).toHaveBeenCalledWith({ to: newWorkspaceRoute.to })
})

test('the settings pages replace the workspace sidebar with the settings one', async () => {
  worker.use(list([workspace({})]))
  const { screen } = await renderWithRouter(
    <WorkspaceLayout />,
    routeMount(teamRoute, { workspaceId: 'AcmeWs01' }),
  )

  await expect
    .element(screen.getByRole('link', { name: 'Back to workspace', exact: true }))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Contacts', exact: true }))
    .not.toBeInTheDocument()
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
    .element(screen.getByText(/To appeal, contact your Sphericon administrator/))
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
}

const ACTION_URL = 'https://account.sphericon.test/acme/resolve'
const HOLD_WITH_ACTION = { ...HOLD, key: 'k-action', actionUrl: ACTION_URL }

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

test.each(['owner', 'admin'] as const)(
  'a hold with an action URL links the %s to it',
  async (role) => {
    worker.use(
      list([workspace({ role, permissions: ['holds:resolve'], holds: [HOLD_WITH_ACTION] })]),
    )
    const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

    await expect
      .element(screen.getByRole('link', { name: 'Resolve' }))
      .toHaveAttribute('href', ACTION_URL)
    await expect.element(screen.getByText(/contact an Owner or Admin/)).not.toBeInTheDocument()
  },
)

test('a hold with an action URL tells a plain member to contact an Owner or Admin, without a link', async () => {
  worker.use(list([workspace({ role: 'member', holds: [HOLD_WITH_ACTION] })]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText(/contact an Owner or Admin/)).toBeInTheDocument()
  await expect.element(screen.getByRole('link', { name: 'Resolve' })).not.toBeInTheDocument()
})

test.each(['owner', 'member'] as const)(
  'a hold without an action URL shows no link and no contact line to a %s',
  async (role) => {
    worker.use(list([workspace({ role, holds: [HOLD] })]))
    const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

    await expect.element(screen.getByText('Sending is on hold')).toBeInTheDocument()
    await expect.element(screen.getByRole('link', { name: 'Resolve' })).not.toBeInTheDocument()
    await expect.element(screen.getByText(/contact an Owner or Admin/)).not.toBeInTheDocument()
  },
)

test('an unaffected workspace shows no hold banner', async () => {
  worker.use(list([workspace({})]))
  const { screen } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await expect.element(screen.getByText('Overview', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Sending is on hold')).not.toBeInTheDocument()
})

test('the switcher navigates to the chosen workspace overview', async () => {
  worker.use(list([workspace({}), workspace({ name: 'Beta', id: 'BetaWs02' })]))
  const { screen, navigate } = await renderWithRouter(<WorkspaceLayout />, MOUNT)

  await screen.getByRole('combobox').click()
  await screen.getByRole('option', { name: 'Beta BetaWs02' }).click()

  expect(navigate).toHaveBeenCalledWith({
    to: overviewRoute.to,
    params: { workspaceId: 'BetaWs02' },
  })
})

// TagsProbe stands in for a workspace page whose query the server refuses.
function TagsProbe() {
  useQuery({ ...siteTagsListOptions({ path: { workspaceId: 'AcmeWs01' } }), retry: false })
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
    .element(screen.getByRole('link', { name: 'Set up two-factor authentication or a Passkey' }))
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
    .element(screen.getByText(/Sign in with a Passkey, or set up two-factor authentication/))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Set up two-factor authentication or a Passkey' }))
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

// Story 35: a member holding second_factor:manage, withheld by the requirement, can still lift it.
test('a withheld second_factor:manage holder can turn the requirement off from the blocked screen', async () => {
  const lifted: string[] = []
  let current = workspace({
    role: 'owner',
    permissions: ['second_factor:manage'],
    secondFactorRequiredAt: '2026-03-01T00:00:00Z',
    secondFactorGraceEndsAt: new Date(Date.now() - DAY).toISOString(),
  })
  worker.use(
    handleSiteWorkspacesList(() => HttpResponse.json([current])),
    handleSiteWorkspacesLiftSecondFactorRequirement(({ params }) => {
      lifted.push(params.workspaceId)
      current = workspace({ role: 'owner', permissions: ['second_factor:manage'] })
      return HttpResponse.json(current)
    }),
    handleSiteTagsList(() =>
      lifted.length > 0
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

  await expect.poll(() => lifted).toEqual([current.id])
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
