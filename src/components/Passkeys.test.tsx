import type { AuthenticationResponseJSON, RegistrationResponseJSON } from '@simplewebauthn/browser'
import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'

import {
  handleSiteAuthSigninProviders,
  handleSitePasskeysBeginCheck,
  handleSitePasskeysBeginRegistration,
  handleSitePasskeysFinishCheck,
  handleSitePasskeysFinishRegistration,
  handleSitePasskeysList,
  handleSitePasskeysRemove,
  handleSitePasskeysRename,
} from '../generated/site/msw.gen.ts'
import type { SitePasskeyList } from '../generated/site/types.gen.ts'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { Passkeys } from './Passkeys.tsx'

const registration = vi.hoisted(() => ({
  start: vi.fn<() => Promise<RegistrationResponseJSON>>(),
  check: vi.fn<(args: unknown) => Promise<AuthenticationResponseJSON>>(),
}))
vi.mock('@simplewebauthn/browser', () => ({
  startRegistration: registration.start,
  startAuthentication: registration.check,
}))

const LAPTOP = { id: '1', name: 'My laptop', createdAt: '2026-01-01T00:00:00Z' }
const PHONE = {
  id: '2',
  name: 'My phone',
  createdAt: '2026-01-02T00:00:00Z',
  lastUsedAt: '2026-02-03T00:00:00Z',
}
const OPTIONS = {
  rp: { id: 'sphericon.test', name: 'sphericon' },
  user: { id: 'AQ', name: 'pat@passkey.test', displayName: 'Pat' },
  challenge: 'Y2hhbGxlbmdl',
  pubKeyCredParams: [{ type: 'public-key' as const, alg: -7 }],
}
const ATTESTATION: RegistrationResponseJSON = {
  id: 'abc',
  rawId: 'abc',
  type: 'public-key',
  clientExtensionResults: {},
  response: { clientDataJSON: 'e30', attestationObject: 'e30' },
}

function serve(list: SitePasskeyList) {
  worker.use(
    handleSitePasskeysList({ body: list }),
    handleSiteAuthSigninProviders({ body: { items: [] } }),
  )
}

test('the list shows each Passkey with its last use', async () => {
  serve({ items: [LAPTOP, PHONE], available: true, passwordSet: true, identityProofFresh: false })
  const { screen } = await renderWithRouter(<Passkeys returnTo="/account/security" />)

  await expect.element(screen.getByText('My laptop')).toBeVisible()
  await expect.element(screen.getByText('Never used')).toBeVisible()
  await expect.element(screen.getByText('My phone')).toBeVisible()
})

test('a User with no Passkey is told what one is for', async () => {
  serve({ items: [], available: true, passwordSet: true, identityProofFresh: false })
  const { screen } = await renderWithRouter(<Passkeys returnTo="/account/security" />)

  await expect.element(screen.getByText(/You have no passkeys yet/)).toBeVisible()
})

test('adding a Passkey passes the options to the authenticator and finishes with the password', async () => {
  serve({ items: [], available: true, passwordSet: true, identityProofFresh: false })
  registration.start.mockResolvedValue(ATTESTATION)
  const finished: unknown[] = []
  worker.use(
    handleSitePasskeysBeginRegistration({ body: { options: OPTIONS } }),
    handleSitePasskeysFinishRegistration(async ({ request }) => {
      finished.push(await request.json())
      return HttpResponse.json(LAPTOP)
    }),
  )
  const { screen } = await renderWithRouter(<Passkeys returnTo="/account/security" />)

  await screen.getByRole('button', { name: 'Add a passkey' }).click()
  await screen.getByLabelText(/^Name this passkey/).fill('My laptop')
  await screen.getByLabelText(/^Current password/).fill('secret-password')
  await screen.getByRole('button', { name: 'Continue' }).click()

  await expect
    .poll(() => finished)
    .toEqual([{ name: 'My laptop', credential: ATTESTATION, currentPassword: 'secret-password' }])
  expect(registration.start).toHaveBeenCalledWith({ optionsJSON: OPTIONS })
})

test('a User with no password adds one after a fresh provider login, with no password field', async () => {
  serve({ items: [], available: true, passwordSet: false, identityProofFresh: true })
  registration.start.mockResolvedValue(ATTESTATION)
  const finished: unknown[] = []
  worker.use(
    handleSitePasskeysBeginRegistration({ body: { options: OPTIONS } }),
    handleSitePasskeysFinishRegistration(async ({ request }) => {
      finished.push(await request.json())
      return HttpResponse.json(LAPTOP)
    }),
  )
  const { screen } = await renderWithRouter(<Passkeys returnTo="/account/security" />)

  await screen.getByRole('button', { name: 'Add a passkey' }).click()
  await screen.getByLabelText(/^Name this passkey/).fill('My laptop')
  await screen.getByRole('button', { name: 'Continue' }).click()

  await expect.poll(() => finished).toEqual([{ name: 'My laptop', credential: ATTESTATION }])
})

test('a User with no password and no fresh provider login is sent to sign in again', async () => {
  serve({ items: [], available: true, passwordSet: false, identityProofFresh: false })
  const { screen } = await renderWithRouter(<Passkeys returnTo="/account/security" />)

  await screen.getByRole('button', { name: 'Add a passkey' }).click()

  await expect.element(screen.getByText('Confirm it is you')).toBeVisible()
})

const REQUEST_OPTIONS = { challenge: 'Y2hhbGxlbmdl', rpId: 'sphericon.test' }
const ASSERTION: AuthenticationResponseJSON = {
  id: 'abc',
  rawId: 'abc',
  type: 'public-key',
  clientExtensionResults: {},
  response: { clientDataJSON: 'e30', authenticatorData: 'e30', signature: 'c2ln' },
}

test('a User with no password and a Passkey proves it with a Passkey check', async () => {
  let fresh = false
  worker.use(
    handleSitePasskeysList(() =>
      HttpResponse.json({
        items: [LAPTOP],
        available: true,
        passwordSet: false,
        identityProofFresh: fresh,
      }),
    ),
    handleSiteAuthSigninProviders({ body: { items: [] } }),
  )
  registration.check.mockResolvedValue(ASSERTION)
  const finished: unknown[] = []
  worker.use(
    handleSitePasskeysBeginCheck({ body: { options: REQUEST_OPTIONS } }),
    handleSitePasskeysFinishCheck(async ({ request }) => {
      finished.push(await request.json())
      fresh = true
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(<Passkeys returnTo="/account/security" />)

  await screen.getByRole('button', { name: 'Add a passkey' }).click()
  await screen.getByRole('button', { name: 'Verify with a passkey' }).click()

  await expect.poll(() => finished).toEqual([{ credential: ASSERTION }])
  expect(registration.check).toHaveBeenCalledWith({ optionsJSON: REQUEST_OPTIONS })
  await expect.element(screen.getByLabelText(/^Name this passkey/)).toBeVisible()
})

test('a User with no Passkey is not offered a Passkey check', async () => {
  serve({ items: [], available: true, passwordSet: false, identityProofFresh: false })
  const { screen } = await renderWithRouter(<Passkeys returnTo="/account/security" />)

  await screen.getByRole('button', { name: 'Add a passkey' }).click()

  await expect.element(screen.getByText('Confirm it is you')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Verify with a passkey' }).elements()).toHaveLength(0)
})

test('a refused registration is reported', async () => {
  serve({ items: [], available: true, passwordSet: true, identityProofFresh: false })
  registration.start.mockResolvedValue(ATTESTATION)
  worker.use(
    handleSitePasskeysBeginRegistration({ body: { options: OPTIONS } }),
    handleSitePasskeysFinishRegistration(() =>
      problem(422, 'the passkey could not be registered: start again'),
    ),
  )
  const { screen } = await renderWithRouter(<Passkeys returnTo="/account/security" />)

  await screen.getByRole('button', { name: 'Add a passkey' }).click()
  await screen.getByLabelText(/^Name this passkey/).fill('My laptop')
  await screen.getByLabelText(/^Current password/).fill('secret-password')
  await screen.getByRole('button', { name: 'Continue' }).click()

  await expect.element(screen.getByText('Could not add the passkey')).toBeVisible()
})

test('renaming a Passkey sends the new name', async () => {
  serve({ items: [LAPTOP], available: true, passwordSet: true, identityProofFresh: false })
  const renamed: unknown[] = []
  worker.use(
    handleSitePasskeysRename(async ({ request, params }) => {
      renamed.push({ id: params.id, body: await request.json() })
      return HttpResponse.json({ ...LAPTOP, name: 'My desk' })
    }),
  )
  const { screen } = await renderWithRouter(<Passkeys returnTo="/account/security" />)

  await screen.getByRole('button', { name: 'Rename' }).click()
  await screen.getByLabelText(/^Passkey name/).fill('My desk')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => renamed).toEqual([{ id: '1', body: { name: 'My desk' } }])
})

test('removing a Passkey asks for the password', async () => {
  serve({ items: [LAPTOP], available: true, passwordSet: true, identityProofFresh: false })
  const removed: unknown[] = []
  worker.use(
    handleSitePasskeysRemove(async ({ request, params }) => {
      removed.push({ id: params.id, body: await request.json() })
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(<Passkeys returnTo="/account/security" />)

  await screen.getByRole('button', { name: 'Remove' }).click()
  await screen.getByLabelText(/^Current password/).fill('secret-password')
  await screen.getByRole('button', { name: 'Remove' }).click()

  await expect
    .poll(() => removed)
    .toEqual([{ id: '1', body: { currentPassword: 'secret-password' } }])
})

test('a User with no password and no fresh provider login is sent to sign in again to remove one', async () => {
  serve({ items: [LAPTOP], available: true, passwordSet: false, identityProofFresh: false })
  const { screen } = await renderWithRouter(<Passkeys returnTo="/account/security" />)

  await screen.getByRole('button', { name: 'Remove' }).click()

  await expect.element(screen.getByText('Confirm it is you')).toBeVisible()
})

test('a refused removal is reported', async () => {
  serve({ items: [LAPTOP], available: true, passwordSet: true, identityProofFresh: false })
  worker.use(handleSitePasskeysRemove(() => problem(409, 'this passkey is the last way')))
  const { screen } = await renderWithRouter(<Passkeys returnTo="/account/security" />)

  await screen.getByRole('button', { name: 'Remove' }).click()
  await screen.getByLabelText(/^Current password/).fill('secret-password')
  await screen.getByRole('button', { name: 'Remove' }).click()

  await expect.element(screen.getByText('Could not remove the passkey')).toBeVisible()
})

test('an Instance without a host name shows no Passkeys', async () => {
  serve({ items: [], available: false, passwordSet: true, identityProofFresh: false })
  const { screen } = await renderWithRouter(<Passkeys returnTo="/account/security" />)

  await expect.element(screen.getByText('Passkeys')).not.toBeInTheDocument()
})
