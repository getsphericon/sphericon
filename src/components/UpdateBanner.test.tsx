import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteInstanceGetState } from '../generated/site/msw.gen.ts'
import type { SiteInstanceState } from '../generated/site/types.gen.ts'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { UpdateBanner } from './UpdateBanner.tsx'

const state = (over: Partial<SiteInstanceState> = {}): SiteInstanceState => ({
  telemetry: { enabled: true, locked: false },
  noticeAcknowledged: true,
  ...over,
})

test('an owner is told which release is newer', async () => {
  worker.use(handleSiteInstanceGetState(() => HttpResponse.json(state({ latestRelease: '0.4.0' }))))
  const { screen } = await renderWithRouter(<UpdateBanner />)

  await expect.element(screen.getByText('A new version is available')).toBeInTheDocument()
  await expect.element(screen.getByText(/Version 0\.4\.0 of sphericon/)).toBeInTheDocument()
})

test('no banner without a newer release', async () => {
  worker.use(handleSiteInstanceGetState({ body: state() }))
  const { screen } = await renderWithRouter(<UpdateBanner />)

  await expect.poll(() => screen.getByText('A new version is available').query()).toBeNull()
})

test('someone who is not an owner of the oldest workspace sees no banner', async () => {
  worker.use(handleSiteInstanceGetState(() => problem(403)))
  const { screen } = await renderWithRouter(<UpdateBanner />)

  await expect.poll(() => screen.getByText('A new version is available').query()).toBeNull()
})
