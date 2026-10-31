import { useQueryClient } from '@tanstack/react-query'
import { expect, test } from 'vitest'

import { siteUserGetMeQueryKey } from '../generated/site/@tanstack/react-query.gen.ts'
import type { SiteUserResource } from '../generated/site/types.gen.ts'
import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { formatDateTime } from '../utils/datetime.ts'
import { useDateFormat } from './useDateFormat.ts'

const AT = '2026-03-08T00:00:00Z'
const LOAD = 'Load profile'

const me: SiteUserResource = {
  id: '1',
  name: 'John',
  email: 'info@getsphericon.com',
  emailVerified: true,
  timezone: 'Asia/Tokyo',
  signupSurveyDue: false,
  acceptancesPending: [],
  createdAt: '2026-01-01T00:00:00Z',
}

function Harness() {
  const queryClient = useQueryClient()
  const { formatDateTime: format } = useDateFormat()
  return (
    <>
      <output>{format(AT)}</output>
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

// Without a cached profile the browser's zone holds; once the profile is in the cache its User
// time zone (ADR 0068) does, and the callers re-render.
test('formats in the browser zone until the cached profile names a zone', async () => {
  const screen = await renderWithProviders(<Harness />)

  await expect.element(screen.getByRole('status')).toHaveTextContent(formatDateTime(AT))

  await screen.getByRole('button', { name: LOAD }).click()

  await expect
    .element(screen.getByRole('status'))
    .toHaveTextContent(formatDateTime(AT, 'Asia/Tokyo'))
  expect(formatDateTime(AT, 'Asia/Tokyo')).toContain('9:00')
})
