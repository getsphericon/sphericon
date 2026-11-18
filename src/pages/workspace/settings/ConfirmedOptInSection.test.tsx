import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteWorkspacesGetConfirmationEmail,
  handleSiteWorkspacesList,
  handleSiteWorkspacesSetConfirmedOptIn,
} from '../../../generated/site/msw.gen.ts'
import type {
  SiteConfirmationEmailResource,
  SiteSetConfirmedOptInInput,
  SiteWorkspaceResource,
} from '../../../generated/site/types.gen.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { worker } from '../../../test/worker.ts'
import { ConfirmedOptInSection } from './ConfirmedOptInSection.tsx'

const workspace = (over: Partial<SiteWorkspaceResource> = {}): SiteWorkspaceResource => ({
  id: 'AcmeWs01',
  name: 'Acme',
  collectKey: 'ck',
  revision: 3,
  postalAddress: '',
  requireConfirmedOptIn: false,
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions: ['workspace:update'],
  mcpUrl: 'https://sphericon.test',
  trackerUrl: 'https://t.sphericon.test',
  shortLinkLength: 35,
  smsMaxLength: { gsm7: 1600, ucs2: 700 },
  holds: [],
  role: 'admin',
  createdAt: '2026-01-01T00:00:00Z',
  ...over,
})

const email = (
  over: Partial<SiteConfirmationEmailResource> = {},
): SiteConfirmationEmailResource => ({
  fromEmail: null,
  fromName: null,
  subject: null,
  intro: null,
  defaults: {
    fromName: 'Acme',
    subject: 'Confirm your subscription to Acme',
    intro: 'You asked to receive email from Acme.',
  },
  revision: 3,
  ...over,
})

test('confirmed opt-in cannot be turned on before the confirmation email is set up', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace()] }),
    handleSiteWorkspacesGetConfirmationEmail({ body: email() }),
  )

  const { screen } = await renderWithRouter(<ConfirmedOptInSection workspace={workspace()} />)

  await expect
    .element(screen.getByRole('switch', { name: 'Require confirmed opt-in' }))
    .toBeDisabled()
  await expect
    .element(screen.getByText('Set up the confirmation email below before turning this on.'))
    .toBeInTheDocument()
  await expect
    .element(screen.getByPlaceholder('Confirm your subscription to Acme'))
    .toBeInTheDocument()
})

test('turning confirmed opt-in on asks the person to affirm the list consented', async () => {
  const sent: SiteSetConfirmedOptInInput[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [workspace()] }),
    handleSiteWorkspacesGetConfirmationEmail({ body: email({ fromEmail: 'hello@mail.acme.com' }) }),
    handleSiteWorkspacesSetConfirmedOptIn(async ({ request }) => {
      sent.push(await request.json())
      return HttpResponse.json(workspace({ requireConfirmedOptIn: true }))
    }),
  )

  const { screen } = await renderWithRouter(<ConfirmedOptInSection workspace={workspace()} />)

  await screen.getByRole('switch', { name: 'Require confirmed opt-in' }).click()
  await expect
    .element(screen.getByText(/every address you can email now agreed to receive your email/))
    .toBeInTheDocument()
  expect(sent).toEqual([])
  await screen.getByRole('button', { name: 'Turn on' }).click()

  await expect.poll(() => sent).toEqual([{ revision: 3, enabled: true }])
})

test('a member sees whether confirmed opt-in is on and cannot change it', async () => {
  worker.use(handleSiteWorkspacesList({ body: [workspace()] }))

  const { screen } = await renderWithRouter(
    <ConfirmedOptInSection
      workspace={workspace({ permissions: [], role: 'member', requireConfirmedOptIn: true })}
    />,
  )

  await expect
    .element(screen.getByText('On: email reaches an address only once it confirms.'))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('switch', { name: 'Require confirmed opt-in' }))
    .not.toBeInTheDocument()
})
