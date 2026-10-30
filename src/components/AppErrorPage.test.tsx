import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteSchemaApplyMigrations } from '../generated/site/msw.gen.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
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
  await expect.element(screen.getByText('503 Service Unavailable')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('button', { name: 'Copy mise run db:migrate' }))
    .toBeInTheDocument()
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

const migrationsPending = {
  status: 503,
  title: 'Service Unavailable',
  code: 'migrations_pending',
  detail: 'The database is 2 migration(s) behind this build. Run `mise run db:migrate`.',
}

test('a server whose migrations are pending offers to run them and says why they did not', async () => {
  const requests: string[] = []
  worker.use(
    handleSiteSchemaApplyMigrations(({ request }) => {
      requests.push(request.method)
      return HttpResponse.json(
        { status: 500, title: 'Internal Server Error', detail: 'migration 42 failed' },
        { status: 500 },
      )
    }),
  )
  const { screen } = await renderWithRouter(<AppErrorPage error={migrationsPending} />)

  await screen.getByRole('button', { name: 'Run pending migrations' }).click()

  await expect.poll(() => requests).toEqual(['POST'])
  await expect.element(screen.getByText('migration 42 failed')).toBeInTheDocument()
})

test('offers no migrations for any other error', async () => {
  const { screen } = await renderWithRouter(
    <AppErrorPage error={{ ...migrationsPending, code: undefined }} />,
  )

  await expect.element(screen.getByRole('button', { name: 'Reload' })).toBeInTheDocument()
  await expect
    .element(screen.getByRole('button', { name: 'Run pending migrations' }))
    .not.toBeInTheDocument()
})
