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
  SiteInvitationResource,
  SiteMembershipResource,
  SiteMembershipRole,
} from '../../generated/site/types.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { MembersSection } from './MembersSection.tsx'

const SLUG = 'test'

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
  const { screen } = await renderWithRouter(<MembersSection slug={SLUG} />)

  await expect.element(screen.getByText('ann@example.com')).toBeInTheDocument()
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
  const { screen } = await renderWithRouter(<MembersSection slug={SLUG} />)

  await screen
    .getByLabelText(/^Email/)
    .last()
    .fill(' new@example.com ')
  await screen.getByRole('button', { name: 'Invite' }).click()

  await expect.element(screen.getByText('https://app.test/invite/abc')).toBeInTheDocument()
  expect(bodies).toEqual([{ email: 'new@example.com', role: 'member' }])

  // The notice can be dismissed.
  await screen.getByRole('button', { name: 'Copy' }).click()
  await screen.getByRole('alert').getByRole('button').last().click()
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
  const { screen } = await renderWithRouter(<MembersSection slug={SLUG} />)

  await screen.getByRole('button', { name: 'Revoke' }).click()
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
  const { screen } = await renderWithRouter(<MembersSection slug={SLUG} />)

  await screen.getByRole('button', { name: 'Remove' }).click()
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
  const { screen } = await renderWithRouter(<MembersSection slug={SLUG} />)

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
  const { screen } = await renderWithRouter(<MembersSection slug={SLUG} />)

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
  const { screen } = await renderWithRouter(<MembersSection slug={SLUG} />)

  await screen.getByRole('button', { name: 'Reset 2FA' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Reset' }).click()

  await expect.poll(() => reset).toBe(true)
})

test('a member is not offered a Second factor reset', async () => {
  worker.use(...teamWithSam('member'))
  const { screen } = await renderWithRouter(<MembersSection slug={SLUG} />)

  await expect.element(screen.getByText('sam@example.com')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Reset 2FA' })).not.toBeInTheDocument()
})
