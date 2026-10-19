import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteInstanceGetState } from '../generated/site/msw.gen.ts'
import type { SiteInstanceState } from '../generated/site/types.gen.ts'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { LicenseBanner } from './LicenseBanner.tsx'

const state = (over: Partial<SiteInstanceState> = {}): SiteInstanceState => ({
  telemetry: { enabled: true, locked: false },
  noticeAcknowledged: true,
  telemetrySchemaUrl: 'https://docs.example.test/telemetry-schema',
  ...over,
})

test('an owner is told the license key expired and when', async () => {
  worker.use(
    handleSiteInstanceGetState(() =>
      HttpResponse.json(state({ licenseExpiredAt: '2026-05-20T12:00:00Z' })),
    ),
  )
  const { screen } = await renderWithRouter(<LicenseBanner />)

  await expect.element(screen.getByText('The license key has expired')).toBeInTheDocument()
  await expect.element(screen.getByText(/expired on May 20, 2026/)).toBeInTheDocument()
})

test('no banner while the license key has not lapsed', async () => {
  worker.use(handleSiteInstanceGetState({ body: state() }))
  const { screen } = await renderWithRouter(<LicenseBanner />)

  await expect.poll(() => screen.getByText('The license key has expired').query()).toBeNull()
})

test('someone who is not an owner of the oldest workspace sees no banner', async () => {
  worker.use(handleSiteInstanceGetState(() => problem(403)))
  const { screen } = await renderWithRouter(<LicenseBanner />)

  await expect.poll(() => screen.getByText('The license key has expired').query()).toBeNull()
})
