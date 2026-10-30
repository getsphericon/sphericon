import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'

import {
  handleSiteAuthSigninProviders,
  handleSiteLegalDocumentsSignupItems,
  handleSitePublicInvitationsAccept,
  handleSitePublicInvitationsLookup,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import { SignupItemKind } from '../../generated/site/types.gen.ts'
import type { SiteInvitationLookupResult } from '../../generated/site/types.gen.ts'
import { Route as acceptanceRoute } from '../../routes/acceptance.tsx'
import { Route as acceptInvitationRoute } from '../../routes/invitations/$token.tsx'
import { Route as loginRoute } from '../../routes/login.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AcceptInvitationPage } from './accept.tsx'

const MOUNT = routeMount(acceptInvitationRoute, { token: 'inv-1' })

const invite: SiteInvitationLookupResult = { workspaceName: 'Acme', email: 'ada@example.com' }

test('a signed-out invitee supplies a name and password to accept', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteWorkspacesList(() => problem(401)),
    handleSitePublicInvitationsLookup({ body: invite }),
    handleSitePublicInvitationsAccept(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({})
    }),
  )
  const { screen, navigate } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await expect
    .element(screen.getByText("You've been invited to join Acme on Sphericon."))
    .toBeInTheDocument()
  await expect.element(screen.getByText('ada@example.com')).toBeInTheDocument()
  await screen.getByLabelText(/^Your name/).fill('  Ada  ')
  await screen.getByLabelText(/^Password/).fill('s3cret')
  await screen.getByRole('button', { name: 'Accept invitation' }).click()

  await expect.element(screen.getByText('Invitation accepted')).toBeInTheDocument()
  expect(bodies).toEqual([{ name: 'Ada', password: 's3cret' }])
  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: loginRoute.to }])
})

test('a signed-in invitee can join with an empty body', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [] }),
    handleSitePublicInvitationsLookup({ body: invite }),
    handleSitePublicInvitationsAccept(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({})
    }),
  )
  const { screen } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await screen.getByRole('button', { name: 'Join workspace' }).click()

  await expect.element(screen.getByText('Invitation accepted')).toBeInTheDocument()
  await expect.poll(() => bodies).toEqual([{}])
})

test('a signed-out invitee with an account is sent to sign in and back to the accept page', async () => {
  worker.use(
    handleSitePublicInvitationsLookup({ body: invite }),
    handleSiteWorkspacesList(() => problem(401)),
  )
  const { screen, navigate } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await expect
    .element(screen.getByRole('button', { name: 'Join workspace' }))
    .not.toBeInTheDocument()
  await screen.getByRole('link', { name: 'Sign in' }).click()

  await expect
    .poll(() => navigate.mock.calls)
    .toContainEqual([
      expect.objectContaining({
        to: loginRoute.to,
        search: { redirect: expect.stringContaining('inv-1') },
      }),
    ])
})

test('an accept failure shows the API error and stays on the page', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [] }),
    handleSitePublicInvitationsLookup({ body: invite }),
    handleSitePublicInvitationsAccept(() => problem(410, { detail: 'Invitation revoked' })),
  )
  const { screen, navigate } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await screen.getByRole('button', { name: 'Join workspace' }).click()

  await expect.element(screen.getByText('Invitation revoked')).toBeInTheDocument()
  expect(navigate).not.toHaveBeenCalled()
})

test('shows a loading state while the invitation is looked up', async () => {
  worker.use(
    handleSitePublicInvitationsLookup(async () => new Promise<Response>(vi.fn<() => void>())),
  )
  const { screen } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await expect.element(screen.getByText('Loading invitation…')).toBeInTheDocument()
})

test('an unknown invitation shows the invalid alert', async () => {
  worker.use(handleSitePublicInvitationsLookup(() => problem(404)))
  const { screen } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await expect
    .element(screen.getByText('This invitation is invalid or has expired.'))
    .toBeInTheDocument()
})

test('an invitation offers each enabled provider and hands the provider flow its token', async () => {
  worker.use(
    handleSiteWorkspacesList(() => problem(401)),
    handleSitePublicInvitationsLookup({ body: invite }),
    handleSiteAuthSigninProviders({ body: { items: [{ name: 'google', displayName: 'Google' }] } }),
  )
  const { screen } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await expect
    .element(screen.getByRole('link', { name: 'Continue with Google' }))
    .toHaveAttribute('href', '/site/auth/signin/google/start?invitation=inv-1')
})

// The Sign-up items of an Instance that has them (ADR 0062): one of each kind.
const SIGNUP_ITEMS = [
  {
    key: 'terms',
    title: 'Terms of service',
    kind: SignupItemKind.REQUIRED_ACCEPTANCE,
    version: '2026-02',
  },
  { key: 'news', title: 'News from us', kind: SignupItemKind.OPTIONAL_CONSENT, version: '2026-01' },
  {
    key: 'data-notice',
    title: 'How we use your data',
    kind: SignupItemKind.NOTICE,
    version: '2026-01',
  },
]

test('an invitee who sets up an account answers the Sign-up items, each unticked at first', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteWorkspacesList(() => problem(401)),
    handleSitePublicInvitationsLookup({ body: invite }),
    handleSiteLegalDocumentsSignupItems({ body: { items: SIGNUP_ITEMS } }),
    handleSitePublicInvitationsAccept(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({})
    }),
  )
  const { screen } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  const terms = screen.getByRole('checkbox', { name: 'Terms of service' })
  await expect.element(terms).not.toBeChecked()
  await expect.element(screen.getByRole('checkbox', { name: 'News from us' })).not.toBeChecked()
  await expect
    .element(screen.getByRole('link', { name: 'How we use your data' }))
    .toHaveAttribute('href', '/legal/data-notice')
  await terms.click()
  await screen.getByLabelText(/^Your name/).fill('Ada')
  await screen.getByLabelText(/^Password/).fill('s3cret')
  await screen.getByRole('button', { name: 'Accept invitation' }).click()

  await expect.element(screen.getByText('Invitation accepted')).toBeInTheDocument()
  expect(bodies).toEqual([
    {
      name: 'Ada',
      password: 's3cret',
      signupItems: [
        { key: 'terms', version: '2026-02', accepted: true },
        { key: 'news', version: '2026-01', accepted: false },
        { key: 'data-notice', version: '2026-01' },
      ],
    },
  ])
})

test('shows the refusal of a required acceptance under its box', async () => {
  worker.use(
    handleSiteWorkspacesList(() => problem(401)),
    handleSitePublicInvitationsLookup({ body: invite }),
    handleSiteLegalDocumentsSignupItems({ body: { items: SIGNUP_ITEMS } }),
    handleSitePublicInvitationsAccept(() =>
      problem(422, {
        detail: 'accept the Terms of service to sign up',
        errors: { 'signupItems.terms': ['accept the Terms of service to sign up'] },
      }),
    ),
  )
  const { screen } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await screen.getByLabelText(/^Your name/).fill('Ada')
  await screen.getByLabelText(/^Password/).fill('s3cret')
  await screen.getByRole('button', { name: 'Accept invitation' }).click()

  await expect
    .element(screen.getByRole('checkbox', { name: 'Terms of service' }))
    .toHaveAccessibleDescription(/accept the Terms of service to sign up/)
})

test('a signed-in invitee joins without answering the Sign-up items', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [] }),
    handleSitePublicInvitationsLookup({ body: invite }),
    handleSiteLegalDocumentsSignupItems({ body: { items: SIGNUP_ITEMS } }),
    handleSitePublicInvitationsAccept(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({})
    }),
  )
  const { screen } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await expect.element(screen.getByRole('checkbox', { name: 'Terms of service' })).toBeVisible()
  await screen.getByRole('button', { name: 'Join workspace' }).click()

  await expect.element(screen.getByText('Invitation accepted')).toBeInTheDocument()
  await expect.poll(() => bodies).toEqual([{}])
})

test('a provider sign-in refused for the invited address says so on the invitation', async () => {
  worker.use(
    handleSiteWorkspacesList(() => problem(401)),
    handleSitePublicInvitationsLookup({ body: invite }),
  )
  const { screen } = await renderWithRouter(<AcceptInvitationPage />, {
    ...MOUNT,
    initialPath: `${MOUNT.initialPath}?error=invitation_email_mismatch`,
  })

  await expect
    .element(screen.getByText(/This invitation is for a different email address/))
    .toBeInTheDocument()
})

test('a signed-in invitee with an acceptance pending is sent to the acceptance screen, to come back', async () => {
  worker.use(
    handleSiteWorkspacesList(() => problem(403, { code: 'acceptance_pending' })),
    handleSitePublicInvitationsLookup({ body: invite }),
  )
  const { navigate } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await expect
    .poll(() => navigate.mock.calls)
    .toContainEqual([{ to: acceptanceRoute.to, search: { redirect: MOUNT.initialPath } }])
})
