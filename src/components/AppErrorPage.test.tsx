import { expect, test } from 'vitest'

import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { AppErrorPage } from './AppErrorPage.tsx'

test('shows what the server says is wrong', async () => {
  const error = {
    status: 503,
    title: 'Service Unavailable',
    detail: 'The database is 2 migration(s) behind this build. Run `mise run db:migrate`.',
  }
  const { screen } = await renderWithRouter(<AppErrorPage error={error} />)

  await expect
    .element(screen.getByRole('heading', { name: 'Something went wrong' }))
    .toBeInTheDocument()
  await expect.element(screen.getByText(/mise run db:migrate/)).toBeInTheDocument()
})

test('falls back to a plain sentence for an error with no problem body', async () => {
  const { screen } = await renderWithRouter(<AppErrorPage error={new Error('boom')} />)

  await expect.element(screen.getByText('The page could not be loaded.')).toBeInTheDocument()
})
