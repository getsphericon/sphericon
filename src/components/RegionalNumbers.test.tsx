import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { expect, test } from 'vitest'

import { siteUserGetMeQueryKey } from '../generated/site/@tanstack/react-query.gen.ts'
import type { SiteUserResource } from '../generated/site/types.gen.ts'
import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { RegionalNumbers } from './RegionalNumbers.tsx'

const LOAD = 'Load profile'

const me: SiteUserResource = {
  id: '1',
  name: 'John',
  email: 'info@getsphericon.com',
  emailVerified: true,
  timezone: null,
  regionalFormat: 'de-DE',
  signupSurveyDue: false,
  acceptancesPending: [],
  createdAt: '2026-01-01T00:00:00Z',
}

function Harness({ count, instance = false }: { count: number; instance?: boolean }) {
  const queryClient = useQueryClient()
  const { t } = useTranslation()
  return (
    <>
      <RegionalNumbers instance={instance} />
      <output>{t(($) => $.segments.previewCount, { count })}</output>
      <button
        type="button"
        onClick={() => {
          queryClient.setQueryData(siteUserGetMeQueryKey(), me)
        }}
      >
        {LOAD}
      </button>
    </>
  )
}

// A number a string prints follows the User's Regional format (#1022): the Instance's language
// holds until the cached profile names one, then every string already on screen is written in it
// without a reload.
test('a number in a string follows the cached Regional format', async () => {
  const screen = await renderWithProviders(<Harness count={1_234_567} />)

  await expect.element(screen.getByRole('status')).toHaveTextContent('1,234,567 matching contacts')

  await screen.getByRole('button', { name: LOAD }).click()

  await expect.element(screen.getByRole('status')).toHaveTextContent('1.234.567 matching contacts')
})

// The plural form is picked from the UI language's rules, never from the Regional format: with a
// de-DE format a count of one is still "contact" and not "contacts".
test('the plural form still follows the UI language', async () => {
  const screen = await renderWithProviders(<Harness count={1} />)

  await screen.getByRole('button', { name: LOAD }).click()

  await expect.element(screen.getByRole('status')).toHaveTextContent('1 matching contact')
})

// The Staff console has no User preference: pinned to the Instance, the numbers stay in its
// language even with a Regional format in the cache.
test('a pinned page keeps the Instance language', async () => {
  const screen = await renderWithProviders(<Harness count={1_234_567} instance />)

  await screen.getByRole('button', { name: LOAD }).click()

  await expect.element(screen.getByRole('status')).toHaveTextContent('1,234,567 matching contacts')
})
