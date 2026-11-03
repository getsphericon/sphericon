import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteSecondFactorGetStatus,
  handleSiteUserGetMe,
  handleSiteUserUpdateMe,
  handleSiteWorkspacesListDeleted,
} from '../../generated/site/msw.gen.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { ProfilePage } from './profile.tsx'

const user = {
  id: '1',
  name: 'John',
  email: 'info@getsphericon.com',
  emailVerified: true,
  timezone: null,
  regionalFormat: null,
  signupSurveyDue: false,
  acceptancesPending: [],
  createdAt: '2026-01-01T00:00:00Z',
}

// serve answers the profile page's reads and records each profile update's body.
function serve(puts: string[]) {
  worker.use(
    handleSiteUserGetMe({ body: user }),
    handleSiteSecondFactorGetStatus({
      body: {
        enabled: false,
        pending: false,
        recoveryCodesRemaining: 0,
        passwordSet: true,
        identityProofFresh: false,
      },
    }),
    handleSiteWorkspacesListDeleted({ body: [] }),
    handleSiteUserUpdateMe(async ({ request }) => {
      puts.push(await request.clone().text())
      return HttpResponse.json({ ...user, name: 'Renamed' })
    }),
  )
}

test('loads the profile and submits a name change', async () => {
  const puts: string[] = []
  serve(puts)

  const { screen } = await renderWithRouter(<ProfilePage />)

  const nameInput = screen.getByLabelText(/^Name/)
  await expect.element(nameInput).toHaveValue('John')

  await nameInput.fill('Renamed')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => puts)
    .toEqual(['{"name":"Renamed","timezone":null,"regionalFormat":null}'])
})

// The User time zone (ADR 0068) is empty until chosen, and then sent with the profile.
test('submits a chosen time zone', async () => {
  const puts: string[] = []
  serve(puts)

  const { screen } = await renderWithRouter(<ProfilePage />)

  const zone = screen.getByRole('combobox', { name: 'Time zone' })
  await expect.element(zone).toHaveValue('')
  await zone.fill('Europe/Berl')
  await screen.getByRole('option', { name: 'Europe/Berlin' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => puts)
    .toEqual(['{"name":"John","timezone":"Europe/Berlin","regionalFormat":null}'])
})

// The Regional format (#1010) is empty until chosen, offered as language-region pairs named in the
// language of the UI, and sent with the profile; clearing it sends null again.
test('submits a chosen Regional format', async () => {
  const puts: string[] = []
  serve(puts)

  const { screen } = await renderWithRouter(<ProfilePage />)

  const format = screen.getByRole('combobox', { name: 'Regional format' })
  await expect.element(format).toHaveValue('')
  await format.fill('German (Germany)')
  await screen.getByRole('option', { name: /^German \(Germany\) · 31\.12\.2026$/ }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => puts)
    .toEqual(['{"name":"John","timezone":null,"regionalFormat":"de-DE"}'])
})
