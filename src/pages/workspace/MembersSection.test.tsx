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
  handleSiteWorkspacesLeave,
} from '../../generated/site/msw.gen.ts'
import type {
  SiteInvitationResource,
  SiteMembershipResource,
  SiteMembershipRole,
} from '../../generated/site/types.gen.ts'
import { Route as indexRoute } from '../../routes/index.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { MembersSection } from './MembersSection.tsx'

const WORKSPACE_ID = 'TestWs01'

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

// The signed-in User: Ann, the member above (the Second factor reset is role-gated).
const ME = {
  id: '10',
  name: 'Ann',
  email: 'ann@example.com',
  emailVerified: true,
  createdAt: '2026-01-01T00:00:00Z',
}
const me = () => handleSiteUserGetMe({ body: ME })
const members = () => [me(), handleSiteMembershipsList({ body: [member] })]
const invites = (items: SiteInvitationResource[]) => handleSiteInvitationsList({ body: items })

test('lists members and pending invitations', async () => {
  worker.use(...members(), invites([invitation]))
  const { screen } = await renderWithRouter(<MembersSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText('ann@example.com')).toBeInTheDocument()
  await screen.getByRole('tab', { name: /Pending invitations/ }).click()
  await expect.element(screen.getByText('new@example.com')).toBeInTheDocument()
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
  const { screen } = await renderWithRouter(<MembersSection workspaceId={WORKSPACE_ID} />)

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
  const { screen } = await renderWithRouter(<MembersSection workspaceId={WORKSPACE_ID} />)

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
  const { screen } = await renderWithRouter(<MembersSection workspaceId={WORKSPACE_ID} />)

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
  const { screen } = await renderWithRouter(<MembersSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('combobox').first().click()
  await screen.getByRole('option', { name: 'admin' }).first().click()

  await expect.poll(() => bodies).toEqual([{ role: 'admin' }])
})

test('shows an error alert when members fail to load', async () => {
  worker.use(
    me(),
    handleSiteMembershipsList(() => problem(500, { detail: 'boom' })),
    invites([]),
  )
  const { screen } = await renderWithRouter(<MembersSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText('Failed to load members').first()).toBeInTheDocument()
})

// The signed-in User (id 10) with the given role, next to Sam, who has a Second factor.
function teamWithSam(role: SiteMembershipRole) {
  const sam: SiteMembershipResource = {
    ...member,
    id: '2',
    userId: '11',
    email: 'sam@example.com',
    name: 'Sam',
  }
  return [me(), handleSiteMembershipsList({ body: [{ ...member, role }, sam] }), invites([])]
}

test('an owner resets a member Second factor after confirmation', async () => {
  let reset = false
  worker.use(
    ...teamWithSam('owner'),
    handleSiteMembershipsResetSecondFactor(({ params }) => {
      reset = params.id === '2'
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(<MembersSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'More actions' }).last().click()
  await screen.getByRole('menuitem', { name: 'Reset 2FA' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Reset' }).click()

  await expect.poll(() => reset).toBe(true)
})

test('a member is not offered a Second factor reset', async () => {
  worker.use(...teamWithSam('member'))
  const { screen } = await renderWithRouter(<MembersSection workspaceId={WORKSPACE_ID} />)

  await expect.element(screen.getByText('sam@example.com')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'More actions' }).last().click()
  await expect.element(screen.getByRole('menuitem', { name: 'Remove' })).toBeInTheDocument()
  await expect.element(screen.getByRole('menuitem', { name: 'Reset 2FA' })).not.toBeInTheDocument()
})

test('leaves the workspace after confirmation', async () => {
  let left = false
  worker.use(
    ...members(),
    invites([]),
    handleSiteWorkspacesLeave(() => {
      left = true
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<MembersSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'Leave workspace' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Leave' }).click()

  await expect.poll(() => left).toBe(true)
  // The index route picks the next Workspace, or the create screen when none is left.
  await expect.poll(() => navigate).toHaveBeenCalledWith({ to: indexRoute.to })
})

test('keeps the workspace and says why when the only owner tries to leave', async () => {
  worker.use(
    ...members(),
    invites([]),
    handleSiteWorkspacesLeave(() =>
      HttpResponse.json(
        {
          title: 'cannot leave as the only owner',
          detail:
            'you are the only owner: make another member an owner or delete the workspace first',
          status: 422,
        },
        { status: 422, headers: { 'content-type': 'application/problem+json' } },
      ),
    ),
  )
  const { screen } = await renderWithRouter(<MembersSection workspaceId={WORKSPACE_ID} />)

  await screen.getByRole('button', { name: 'Leave workspace' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Leave' }).click()

  await expect.element(screen.getByText(/make another member an owner/)).toBeInTheDocument()
})
