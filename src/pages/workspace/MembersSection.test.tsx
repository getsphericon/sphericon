import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteInvitationsCreate,
  handleSiteInvitationsDelete,
  handleSiteInvitationsList,
  handleSiteMembershipsDelete,
  handleSiteMembershipsList,
  handleSiteMembershipsResetSecondFactor,
  handleSiteMembershipsUpdate,
  handleSiteUserGetMe,
} from '../../generated/site/msw.gen.ts'
import type {
  Permission,
  SiteInvitationResource,
  SiteMembershipResource,
  SiteWorkspaceResource,
} from '../../generated/site/types.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { MembersSection } from './MembersSection.tsx'

// The Workspace as the signed-in User sees it: managing the members needs members:manage and
// resetting a Second factor second_factor:manage.
const workspaceWith = (permissions: Permission[]): SiteWorkspaceResource => ({
  id: 'TestWs01',
  name: 'Acme',
  collectKey: 'omck_test_key',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions,
  mcpUrl: 'https://sphericon.test',
  trackerUrl: 'https://t.sphericon.test',
  holds: [],
  role: 'member',
  createdAt: '2026-01-01T00:00:00Z',
})
const workspace = workspaceWith(['members:manage'])

const member: SiteMembershipResource = {
  id: '1',
  userId: '10',
  email: 'ann@example.com',
  name: 'Ann',
  role: 'member',
  createdAt: '2026-01-01T00:00:00Z',
}

const invitation: SiteInvitationResource = {
  id: '5',
  email: 'new@example.com',
  role: 'admin',
  expiresAt: '2026-02-01T00:00:00Z',
  createdAt: '2026-01-01T00:00:00Z',
}

// The signed-in User: Ann, the member above (nobody resets their own Second factor here).
const ME = {
  id: '10',
  name: 'Ann',
  email: 'ann@example.com',
  emailVerified: true,
  signupSurveyDue: false,
  acceptancesPending: [],
  createdAt: '2026-01-01T00:00:00Z',
}
const me = () => handleSiteUserGetMe({ body: ME })
const members = () => [me(), handleSiteMembershipsList({ body: [member] })]
const invites = (items: SiteInvitationResource[]) => handleSiteInvitationsList({ body: items })

test('lists members and pending invitations', async () => {
  worker.use(...members(), invites([invitation]))
  const { screen } = await renderWithRouter(<MembersSection workspace={workspace} />)

  await expect.element(screen.getByText('ann@example.com')).toBeInTheDocument()
  await screen.getByRole('tab', { name: /Pending invitations/ }).click()
  await expect.element(screen.getByText('new@example.com')).toBeInTheDocument()
})

test('labels how each member got in and leaves a member from before it unlabelled', async () => {
  const joined: SiteMembershipResource = {
    ...member,
    id: '2',
    userId: '11',
    email: 'dan@example.com',
    name: 'Dan',
    source: 'domain',
  }
  worker.use(me(), handleSiteMembershipsList({ body: [member, joined] }), invites([]))
  const { screen } = await renderWithRouter(<MembersSection workspace={workspace} />)

  await expect.element(screen.getByText('Joined by domain')).toBeInTheDocument()
  await expect.element(screen.getByText('Invited')).not.toBeInTheDocument()
})

test('invites a member and reveals the invite link', async () => {
  const bodies: unknown[] = []
  worker.use(
    ...members(),
    invites([]),
    handleSiteInvitationsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(
        { inviteUrl: 'https://app.test/invite/abc', resource: invitation },
        { status: 201 },
      )
    }),
  )
  const { screen } = await renderWithRouter(<MembersSection workspace={workspace} />)

  await screen.getByRole('button', { name: 'Invite' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByLabelText(/^Email/).fill(' new@example.com ')
  await dialog.getByRole('button', { name: 'Invite' }).click()

  await expect.element(screen.getByText('https://app.test/invite/abc')).toBeInTheDocument()
  expect(bodies).toEqual([{ email: 'new@example.com', role: 'member' }])

  // Closing the modal drops the link.
  await screen.getByRole('button', { name: 'Copy' }).click()
  await dialog.getByRole('button').first().click()
  await expect.element(screen.getByText('https://app.test/invite/abc')).not.toBeInTheDocument()
})

test('revokes an invitation after confirmation', async () => {
  let revoked = false
  worker.use(
    ...members(),
    invites([invitation]),
    handleSiteInvitationsDelete(() => {
      revoked = true
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(<MembersSection workspace={workspace} />)

  await screen.getByRole('tab', { name: /Pending invitations/ }).click()
  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Revoke' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.poll(() => revoked).toBe(true)
})

test('removes a member after confirmation', async () => {
  let removed = false
  worker.use(
    ...members(),
    invites([]),
    handleSiteMembershipsDelete(() => {
      removed = true
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(<MembersSection workspace={workspace} />)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Remove' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.poll(() => removed).toBe(true)
})

test('changes a member role', async () => {
  const bodies: unknown[] = []
  worker.use(
    ...members(),
    invites([]),
    handleSiteMembershipsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ ...member, role: 'admin' })
    }),
  )
  const { screen } = await renderWithRouter(<MembersSection workspace={workspace} />)

  await screen.getByRole('combobox').first().click()
  await screen.getByRole('option', { name: 'admin' }).first().click()

  await expect.poll(() => bodies).toEqual([{ role: 'admin' }])
  await expect.element(screen.getByText('Role updated')).toBeInTheDocument()
})

test('locks the role select with a loader while the change is saved', async () => {
  const gate = Promise.withResolvers<undefined>()
  worker.use(
    ...members(),
    invites([]),
    handleSiteMembershipsUpdate(async () => {
      await gate.promise
      return HttpResponse.json({ ...member, role: 'admin' })
    }),
  )
  const { screen } = await renderWithRouter(<MembersSection workspace={workspace} />)

  await screen.getByRole('combobox').first().click()
  await screen.getByRole('option', { name: 'admin' }).first().click()

  await expect.element(screen.getByRole('combobox').first()).toBeDisabled()
  await expect.element(screen.getByRole('combobox').first()).toHaveValue('admin')
  gate.resolve(undefined)
  await expect.element(screen.getByText('Role updated')).toBeInTheDocument()
})

test('shows an error alert when members fail to load', async () => {
  worker.use(
    me(),
    handleSiteMembershipsList(() => problem(500, { detail: 'boom' })),
    invites([]),
  )
  const { screen } = await renderWithRouter(<MembersSection workspace={workspace} />)

  await expect.element(screen.getByText('Failed to load members').first()).toBeInTheDocument()
})

// The signed-in User (id 10) next to Sam, who has a Second factor.
function teamWithSam() {
  const sam: SiteMembershipResource = {
    ...member,
    id: '2',
    userId: '11',
    email: 'sam@example.com',
    name: 'Sam',
  }
  return [me(), handleSiteMembershipsList({ body: [member, sam] }), invites([])]
}

test('a member with second_factor:manage resets another member Second factor', async () => {
  let reset = false
  worker.use(
    ...teamWithSam(),
    handleSiteMembershipsResetSecondFactor(({ params }) => {
      reset = params.id === '2'
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(
    <MembersSection workspace={workspaceWith(['second_factor:manage'])} />,
  )

  await screen.getByRole('button', { name: 'More actions' }).last().click()
  await screen.getByRole('menuitem', { name: 'Reset 2FA' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Reset' }).click()

  await expect.poll(() => reset).toBe(true)
})

test('a member without second_factor:manage is not offered a Second factor reset', async () => {
  worker.use(...teamWithSam())
  const { screen } = await renderWithRouter(<MembersSection workspace={workspace} />)

  await expect.element(screen.getByText('sam@example.com')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'More actions' }).last().click()
  await expect.element(screen.getByRole('menuitem', { name: 'Remove' })).toBeInTheDocument()
  await expect.element(screen.getByRole('menuitem', { name: 'Reset 2FA' })).not.toBeInTheDocument()
})
