import { expect, test } from 'vitest'

import type { SiteProviderLimitStatus } from '../../generated/site/types.gen.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { ProviderLimitStatus } from './ProviderLimitStatus.tsx'

const status = (over: Partial<SiteProviderLimitStatus>): SiteProviderLimitStatus => ({
  perSecond: { limit: null, source: null },
  perDay: { limit: null, source: null },
  sentLast24h: 0,
  warnings: [],
  ...over,
})

test('an unlimited window says so and shows no usage bar', async () => {
  const { screen } = await renderWithRouter(<ProviderLimitStatus status={status({})} />)

  await expect.element(screen.getByText(/Unlimited/).first()).toBeInTheDocument()
  expect(screen.container.querySelector('[role="progressbar"]')).toBeNull()
})

test('each ceiling names its source and the daily one gets a usage bar', async () => {
  const { screen } = await renderWithRouter(
    <ProviderLimitStatus
      status={status({
        perSecond: { limit: 10, source: 'manual' },
        perDay: { limit: 1000, source: 'provider' },
        sentLast24h: 250,
      })}
    />,
  )

  await expect.element(screen.getByText('Manual')).toBeInTheDocument()
  await expect.element(screen.getByText('Provider')).toBeInTheDocument()
  await expect.element(screen.getByRole('progressbar')).toBeInTheDocument()
})

test('labels the row with its warnings only when asked', async () => {
  const flagged = status({ warnings: ['unlimited', 'providerStopped'] })
  const { screen } = await renderWithRouter(
    <>
      <ProviderLimitStatus status={flagged} withWarnings />
      <ProviderLimitStatus status={flagged} />
    </>,
  )

  await expect.element(screen.getByText('Stopped')).toBeInTheDocument()
  expect(screen.getByText('No limit').all()).toHaveLength(1)
})
