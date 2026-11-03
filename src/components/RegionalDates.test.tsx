import { DateInput } from '@mantine/dates'
import { useQueryClient } from '@tanstack/react-query'
import { expect, test, vi } from 'vitest'
import { userEvent } from 'vitest/browser'

import { siteUserGetMeQueryKey } from '../generated/site/@tanstack/react-query.gen.ts'
import type { SiteUserResource } from '../generated/site/types.gen.ts'
import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { RegionalDates } from './RegionalDates.tsx'

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

function Harness({ onChange }: { onChange: (value: string | null) => void }) {
  const queryClient = useQueryClient()
  return (
    <>
      <DateInput aria-label="Day" valueFormat="L" defaultValue="2026-12-31" onChange={onChange} />
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

// A date input below RegionalDates shows the Instance's format until the cached profile names a
// Regional format, then shows and reads dates in it (#1010).
test('a date input shows and accepts dates in the cached Regional format', async () => {
  const onChange = vi.fn<(value: string | null) => void>()
  const screen = await renderWithProviders(
    <RegionalDates>
      <Harness onChange={onChange} />
    </RegionalDates>,
  )
  const input = screen.getByRole('textbox', { name: 'Day' })

  await expect.element(input).toHaveValue('12/31/2026')

  await screen.getByRole('button', { name: LOAD }).click()
  await expect.element(input).toHaveValue('31.12.2026')

  await input.fill('24.12.2026')
  await userEvent.tab()
  await expect.poll(() => onChange.mock.lastCall?.[0]).toBe('2026-12-24')
})
