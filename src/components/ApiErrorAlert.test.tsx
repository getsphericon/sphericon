import { expect, test } from 'vitest'

import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { ApiErrorAlert } from './ApiErrorAlert.tsx'

test('renders the resolved error message and title', async () => {
  const screen = await renderWithProviders(
    <ApiErrorAlert error={{ detail: 'Something broke' }} title="Oops" fallback="fallback" />,
  )

  await expect.element(screen.getByText('Oops')).toBeInTheDocument()
  await expect.element(screen.getByText('Something broke')).toBeInTheDocument()
})

test('shows the fallback when there is no error', async () => {
  const screen = await renderWithProviders(
    <ApiErrorAlert error={null} title="Oops" fallback="Default message" />,
  )

  await expect.element(screen.getByText('Default message')).toBeInTheDocument()
})

test('shows the title alone when the error says nothing and there is no fallback', async () => {
  const screen = await renderWithProviders(
    <ApiErrorAlert error={new TypeError('Failed to fetch')} title="Failed to load activity" />,
  )

  await expect.element(screen.getByText('Failed to load activity')).toBeInTheDocument()
  expect(screen.getByText('Failed to load activity').elements()).toHaveLength(1)
})
