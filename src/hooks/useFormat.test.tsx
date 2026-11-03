import { useQueryClient } from '@tanstack/react-query'
import { expect, test } from 'vitest'

import { siteUserGetMeQueryKey } from '../generated/site/@tanstack/react-query.gen.ts'
import type { SiteUserResource } from '../generated/site/types.gen.ts'
import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { formatDateTime } from '../utils/datetime.ts'
import { useFormat } from './useFormat.ts'

const AT = '2026-03-08T00:00:00Z'
const LOAD = 'Load profile'
// Midday in the browser's own zone, so the day reads the same wherever the test runs.
const NOON = '2026-03-08T12:00:00'
const FIVE_MINUTES_AGO = Date.now() - 5 * 60_000

const me: SiteUserResource = {
  id: '1',
  name: 'John',
  email: 'info@getsphericon.com',
  emailVerified: true,
  timezone: 'Asia/Tokyo',
  regionalFormat: null,
  signupSurveyDue: false,
  acceptancesPending: [],
  createdAt: '2026-01-01T00:00:00Z',
}

function Harness({ profile = me }: { profile?: SiteUserResource }) {
  const queryClient = useQueryClient()
  const { formatDateTime: format } = useFormat()
  return (
    <>
      <output>{format(AT)}</output>
      <button
        type="button"
        onClick={() => {
          queryClient.setQueryData(siteUserGetMeQueryKey(), profile)
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

  await expect.element(screen.getByRole('status')).toHaveTextContent(formatDateTime(AT, 'en'))

  await screen.getByRole('button', { name: LOAD }).click()

  await expect
    .element(screen.getByRole('status'))
    .toHaveTextContent(formatDateTime(AT, 'en', 'Asia/Tokyo'))
  expect(formatDateTime(AT, 'en', 'Asia/Tokyo')).toContain('9:00')
})

// The Regional format (#1010) comes from the same cached profile: the Instance's language holds
// until it names one, then every caller writes dates and times in it, in the User's time zone,
// without a reload.
test('writes dates in the Instance language until the cached profile names a Regional format', async () => {
  const screen = await renderWithProviders(<Harness profile={{ ...me, regionalFormat: 'de-DE' }} />)

  await expect.element(screen.getByRole('status')).toHaveTextContent(formatDateTime(AT, 'en'))

  await screen.getByRole('button', { name: LOAD }).click()

  await expect.element(screen.getByRole('status')).toHaveTextContent('08.03.2026 09:00:00')
})

function Figures() {
  const queryClient = useQueryClient()
  const format = useFormat()
  const figures = [
    format.formatDate(NOON),
    format.formatDay('2026-03-08'),
    format.formatLongDate(NOON),
    format.formatNumber(1_234.5),
    format.formatPercent(0.5),
    format.formatMoney(123_450, 'EUR'),
    // The narrow unit wording is the ICU data's own; the minutes are what matters here.
    format.formatDuration(3_600 + 1_200).includes('20') ? 'duration' : 'no duration',
    format.formatChange(6, 4),
    format.formatPointChange(0.3, 0.2),
    format.formatRelative(FIVE_MINUTES_AGO),
  ]
  return (
    <>
      <output>{figures.join(' | ').replaceAll(/\s/g, ' ')}</output>
      <button
        type="button"
        onClick={() => {
          queryClient.setQueryData(siteUserGetMeQueryKey(), {
            ...me,
            timezone: null,
            regionalFormat: 'de-DE',
          })
        }}
      >
        {LOAD}
      </button>
    </>
  )
}

// Every figure the hook writes follows the Regional format of the cached profile, from dates to
// money and durations.
test('writes every kind of figure in the cached Regional format', async () => {
  const screen = await renderWithProviders(<Figures />)
  const shown = () => screen.getByRole('status').element().textContent

  await expect
    .poll(shown)
    .toBe(
      '03/08/2026 | 03/08/2026 | March 8, 2026 | 1,234.5 | 50% | €1,234.50 | duration | +50% | +10 | 5 minutes ago',
    )

  await screen.getByRole('button', { name: LOAD }).click()

  await expect
    .poll(shown)
    .toBe(
      '08.03.2026 | 08.03.2026 | 8. März 2026 | 1.234,5 | 50 % | 1.234,50 € | duration | +50 % | +10 | vor 5 Minuten',
    )
})
