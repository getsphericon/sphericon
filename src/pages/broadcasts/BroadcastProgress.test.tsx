import { expect, test } from 'vitest'

import { renderWithProviders } from '../../test/renderWithProviders.tsx'
import { BroadcastProgress } from './BroadcastProgress.tsx'

const HOUR_MS = 3_600_000
const DAY_MS = 24 * HOUR_MS

test('shows how many were sent and about how long remains', async () => {
  const screen = await renderWithProviders(
    <BroadcastProgress
      progress={{
        processedCount: 1200,
        remainingCount: 800,
        estimatedCompletionAt: new Date(Date.now() + 3 * HOUR_MS + 60_000).toISOString(),
        limitedBy: 'providerLimit',
      }}
    />,
  )

  await expect.element(screen.getByText('Sent 1200 of 2000')).toBeInTheDocument()
  await expect.element(screen.getByText('about 3 hours remaining')).toBeInTheDocument()
  await expect
    .element(screen.getByText('Paced by the Provider limit of its Integration'))
    .toBeInTheDocument()
})

test('says the Send cap slows it and counts days when the estimate is that far', async () => {
  const screen = await renderWithProviders(
    <BroadcastProgress
      progress={{
        processedCount: 500,
        remainingCount: 1500,
        estimatedCompletionAt: new Date(Date.now() + 3 * DAY_MS + 60_000).toISOString(),
        limitedBy: 'sendCap',
      }}
    />,
  )

  await expect.element(screen.getByText('about 3 days remaining')).toBeInTheDocument()
  await expect
    .element(
      screen.getByText(
        'Slowed by the Send cap set by the platform: the rest goes out as the 24-hour window frees up',
      ),
    )
    .toBeInTheDocument()
  await expect.element(screen.getByText(/Provider limit/)).not.toBeInTheDocument()
})

test('counts minutes when under an hour and says almost done when due', async () => {
  const minutes = await renderWithProviders(
    <BroadcastProgress
      progress={{
        processedCount: 1,
        remainingCount: 9,
        estimatedCompletionAt: new Date(Date.now() + 15 * 60_000 + 5_000).toISOString(),
        limitedBy: 'providerLimit',
      }}
    />,
  )
  await expect.element(minutes.getByText('about 15 minutes remaining')).toBeInTheDocument()

  const due = await renderWithProviders(
    <BroadcastProgress
      progress={{
        processedCount: 9,
        remainingCount: 1,
        estimatedCompletionAt: new Date(Date.now() + 10_000).toISOString(),
        limitedBy: 'providerLimit',
      }}
    />,
  )
  await expect.element(due.getByText('almost done')).toBeInTheDocument()
})

test('shows no estimate when the server has none (a hold)', async () => {
  const screen = await renderWithProviders(
    <BroadcastProgress progress={{ processedCount: 5, remainingCount: 5, limitedBy: null }} />,
  )

  await expect.element(screen.getByText('Sent 5 of 10')).toBeInTheDocument()
  await expect.element(screen.getByText(/remaining/)).not.toBeInTheDocument()
  await expect.element(screen.getByText(/Send cap|Provider limit/)).not.toBeInTheDocument()
})
