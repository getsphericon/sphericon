import { useQueryClient } from '@tanstack/react-query'
import { expect, test } from 'vitest'

import { siteUserGetMeQueryKey } from '../generated/site/@tanstack/react-query.gen.ts'
import type { SiteUserResource } from '../generated/site/types.gen.ts'
import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { usePaginationText } from './usePaginationText.ts'

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

function Harness() {
  const queryClient = useQueryClient()
  const paginationText = usePaginationText()
  return (
    <>
      <output>{paginationText({ from: 1, to: 1_000, totalRecords: 12_345 })}</output>
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

// The range under a table is plain digits to the table itself; here it is written in the User's
// Regional format (#1022), and follows a change of it.
test('writes the range under a table in the cached Regional format', async () => {
  const screen = await renderWithProviders(<Harness />)

  await expect.element(screen.getByRole('status')).toHaveTextContent('1-1,000/12,345')

  await screen.getByRole('button', { name: LOAD }).click()

  await expect.element(screen.getByRole('status')).toHaveTextContent('1-1.000/12.345')
})
