import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteInstanceGetRelease } from '../generated/site/msw.gen.ts'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { UpdateBanner } from './UpdateBanner.tsx'

test('an owner is told which release is newer', async () => {
  worker.use(handleSiteInstanceGetRelease(() => HttpResponse.json({ latestRelease: '0.4.0' })))
  const { screen } = await renderWithRouter(<UpdateBanner />)

  await expect.element(screen.getByText('A new version is available')).toBeInTheDocument()
  await expect.element(screen.getByText(/Version 0\.4\.0 of Sphericon/)).toBeInTheDocument()
})

test('no banner without a newer release', async () => {
  worker.use(handleSiteInstanceGetRelease({ body: {} }))
  const { screen } = await renderWithRouter(<UpdateBanner />)

  await expect.poll(() => screen.getByText('A new version is available').query()).toBeNull()
})

test('someone who owns no workspace sees no banner', async () => {
  worker.use(handleSiteInstanceGetRelease(() => problem(403)))
  const { screen } = await renderWithRouter(<UpdateBanner />)

  await expect.poll(() => screen.getByText('A new version is available').query()).toBeNull()
})
