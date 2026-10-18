import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSitePublicConfirmationsPerform } from '../generated/site/msw.gen.ts'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { ConfirmSubscription } from './confirm.tsx'

test('pressing Confirm performs the confirmation through the contract and thanks the user', async () => {
  const calls: string[] = []
  worker.use(
    handleSitePublicConfirmationsPerform(() => {
      calls.push('performed')
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(<ConfirmSubscription token="tok-1" />)

  await screen.getByRole('button', { name: 'Confirm subscription' }).click()

  await expect.element(screen.getByText('Subscription confirmed')).toBeInTheDocument()
  expect(calls).toEqual(['performed'])
})

test('shows the error state when the token is rejected', async () => {
  worker.use(handleSitePublicConfirmationsPerform(() => problem(400, 'Bad Request')))
  const { screen } = await renderWithRouter(<ConfirmSubscription token="tok-1" />)

  await screen.getByRole('button', { name: 'Confirm subscription' }).click()

  await expect.element(screen.getByText('Confirmation failed')).toBeInTheDocument()
})

test('a link that expires before the button is pressed offers sign-up-again', async () => {
  worker.use(handleSitePublicConfirmationsPerform(() => problem(410, 'Gone')))
  const { screen } = await renderWithRouter(<ConfirmSubscription token="tok-1" />)

  await screen.getByRole('button', { name: 'Confirm subscription' }).click()

  await expect.element(screen.getByText('Link expired')).toBeInTheDocument()
  await expect.element(screen.getByText('Confirmation failed')).not.toBeInTheDocument()
})

test('a rate-limited confirmation shows the localized too-many-requests message', async () => {
  worker.use(handleSitePublicConfirmationsPerform(() => problem(429, 'Too Many Requests')))
  const { screen } = await renderWithRouter(<ConfirmSubscription token="tok-1" />)

  await screen.getByRole('button', { name: 'Confirm subscription' }).click()

  await expect
    .element(screen.getByText('Too many requests. Please wait a minute and try again.'))
    .toBeInTheDocument()
})

test('an expired link offers sign-up-again instead of a button', async () => {
  const { screen } = await renderWithRouter(<ConfirmSubscription expired />)

  await expect.element(screen.getByText('Link expired')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('button', { name: 'Confirm subscription' }))
    .not.toBeInTheDocument()
})
