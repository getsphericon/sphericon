import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteUserGetMe, handleSiteUserUpdateMe } from '../../generated/site/msw.gen.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { ProfilePage } from './profile.tsx'

const user = {
  id: '1',
  name: 'John',
  email: 'info@getsphericon.com',
  emailVerified: true,
  createdAt: '2026-01-01T00:00:00Z',
}

test('loads the profile and submits a name change', async () => {
  const puts: string[] = []
  worker.use(
    handleSiteUserGetMe({ body: user }),
    handleSiteUserUpdateMe(async ({ request }) => {
      puts.push(await request.clone().text())
      return HttpResponse.json({ ...user, name: 'Renamed' })
    }),
  )

  const { screen } = await renderWithRouter(<ProfilePage />)

  const nameInput = screen.getByLabelText(/^Name/)
  await expect.element(nameInput).toHaveValue('John')

  await nameInput.fill('Renamed')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => puts.length).toBeGreaterThan(0)
  expect(puts[0]).toContain('"name":"Renamed"')
})
