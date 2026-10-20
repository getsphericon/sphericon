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

test('shows the fault a development server attaches to a 500', async () => {
  const error = {
    status: 500,
    title: 'Internal Server Error',
    debug: {
      error: 'nil pointer dereference',
      stack: 'goroutine 7 [running]:\nmain.handler()',
      requestId: '01ABC',
      method: 'GET',
      path: '/site/contacts',
    },
  }
  const { screen } = await renderWithRouter(<AppErrorPage error={error} />)

  await expect.element(screen.getByText('nil pointer dereference')).toBeInTheDocument()
  await expect.element(screen.getByText(/goroutine 7/)).toBeInTheDocument()
  await expect.element(screen.getByText(/GET \/site\/contacts/)).toBeInTheDocument()
})
