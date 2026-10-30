import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteOptionalConsentsList,
  handleSiteOptionalConsentsSet,
} from '../../generated/site/msw.gen.ts'
import { SiteOptionalConsentDecision } from '../../generated/site/types.gen.ts'
import type { SiteOptionalConsent } from '../../generated/site/types.gen.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { OptionalConsentsSection } from './OptionalConsentsSection.tsx'

// The optional consents in effect (ADR 0062): one the User gave, one they never answered because
// they signed up before it existed.
const NEWS: SiteOptionalConsent = {
  key: 'news',
  title: 'News from us',
  version: '2026-01',
  decision: SiteOptionalConsentDecision.ACCEPTED,
  decidedAt: '2026-01-05T00:00:00Z',
}
const OFFERS: SiteOptionalConsent = { key: 'offers', title: 'Partner offers', version: '2026-03' }

test('shows each consent ticked only while given, and withdraws one at its version', async () => {
  let consents = [NEWS, OFFERS]
  const changes: { key: string; body: unknown }[] = []
  worker.use(
    handleSiteOptionalConsentsList(() => HttpResponse.json({ items: consents })),
    handleSiteOptionalConsentsSet(async ({ request, params }) => {
      changes.push({ key: params.key, body: await request.json() })
      const withdrawn = { ...NEWS, decision: SiteOptionalConsentDecision.WITHDRAWN }
      consents = [withdrawn, OFFERS]
      return HttpResponse.json(withdrawn)
    }),
  )
  const { screen } = await renderWithRouter(<OptionalConsentsSection />)

  const news = screen.getByRole('checkbox', { name: 'News from us' })
  await expect.element(news).toBeChecked()
  await expect.element(screen.getByRole('checkbox', { name: 'Partner offers' })).not.toBeChecked()
  await expect
    .element(screen.getByRole('link', { name: 'Read the News from us' }))
    .toHaveAttribute('href', '/legal/news')
  await news.click()

  await expect.element(news).not.toBeChecked()
  expect(changes).toEqual([{ key: 'news', body: { version: '2026-01', accepted: false } }])
})
