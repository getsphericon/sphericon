import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSitePublicUnsubscribesPerform } from '../generated/site/msw.gen.ts'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { Unsubscribe } from './unsubscribe.tsx'

test('pressing Unsubscribe performs the opt-out through the contract and offers unsubscribe-all', async () => {
  const calls: string[] = []
  worker.use(
    handleSitePublicUnsubscribesPerform(({ params }) => {
      calls.push(`performed ${params.token}`)
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(
    <Unsubscribe token="tok-1" all="https://app.example/e/u/everything" />,
  )

  await screen.getByRole('button', { name: 'Unsubscribe' }).click()

  await expect
    .element(screen.getByText('You will no longer receive these emails.'))
    .toBeInTheDocument()
  expect(calls).toEqual(['performed tok-1'])
})

test('shows the error state when the token is rejected', async () => {
  worker.use(handleSitePublicUnsubscribesPerform(() => problem(400, 'Bad Request')))
  const { screen } = await renderWithRouter(<Unsubscribe token="tok-1" />)

  await screen.getByRole('button', { name: 'Unsubscribe' }).click()

  await expect.element(screen.getByText('Unsubscribe failed')).toBeInTheDocument()
})
