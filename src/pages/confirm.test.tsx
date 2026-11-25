import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSitePublicConfirmationsPerform,
  handleSitePublicConfirmationsRead,
  handleSitePublicConfirmationsRenew,
} from '../generated/site/msw.gen.ts'
import type { SiteConfirmationPage } from '../generated/site/types.gen.ts'
import { hex } from '../test/payloads.ts'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { ConfirmSubscription } from './confirm.tsx'

const page = (expired: boolean): SiteConfirmationPage => ({
  expired,
  workspaceName: 'Acme',
  brand: {
    accentColor: hex('1a73e8'),
    textColor: hex('202124'),
    backgroundColor: hex('ffffff'),
    bodyFont: 'Arial, sans-serif',
    buttonTextColor: hex('ffffff'),
    buttonRadius: 4,
    logoUrl: '',
  },
})

test('pressing Confirm performs the confirmation through the contract and thanks the user', async () => {
  const calls: string[] = []
  worker.use(
    handleSitePublicConfirmationsRead(() => HttpResponse.json(page(false))),
    handleSitePublicConfirmationsPerform(() => {
      calls.push('performed')
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(<ConfirmSubscription token="tok-1" />)

  await screen.getByRole('button', { name: 'Confirm subscription' }).click()

  await expect.element(screen.getByText('Subscription confirmed')).toBeInTheDocument()
  await expect
    .element(screen.getByText('Thank you — you will receive email from Acme.'))
    .toBeInTheDocument()
  expect(calls).toEqual(['performed'])
})

test('shows the error state when the confirmation fails', async () => {
  worker.use(
    handleSitePublicConfirmationsRead(() => HttpResponse.json(page(false))),
    handleSitePublicConfirmationsPerform(() => problem(500, 'Internal Server Error')),
  )
  const { screen } = await renderWithRouter(<ConfirmSubscription token="tok-1" />)

  await screen.getByRole('button', { name: 'Confirm subscription' }).click()

  await expect.element(screen.getByText('Confirmation failed')).toBeInTheDocument()
})

test('a link that expires before the button is pressed offers a new one', async () => {
  worker.use(
    handleSitePublicConfirmationsRead(() => HttpResponse.json(page(false))),
    handleSitePublicConfirmationsPerform(() => problem(410, 'Gone')),
  )
  const { screen } = await renderWithRouter(<ConfirmSubscription token="tok-1" />)

  await screen.getByRole('button', { name: 'Confirm subscription' }).click()

  await expect.element(screen.getByText('Link expired')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Send a new link' })).toBeInTheDocument()
})

test('a rate-limited confirmation shows the localized too-many-requests message', async () => {
  worker.use(
    handleSitePublicConfirmationsRead(() => HttpResponse.json(page(false))),
    handleSitePublicConfirmationsPerform(() => problem(429, 'Too Many Requests')),
  )
  const { screen } = await renderWithRouter(<ConfirmSubscription token="tok-1" />)

  await screen.getByRole('button', { name: 'Confirm subscription' }).click()

  await expect
    .element(screen.getByText('Too many requests. Please wait a minute and try again.'))
    .toBeInTheDocument()
})

test('an expired link asks for a new one instead of confirming', async () => {
  const calls: string[] = []
  worker.use(
    handleSitePublicConfirmationsRead(() => HttpResponse.json(page(true))),
    handleSitePublicConfirmationsRenew(() => {
      calls.push('renewed')
      return HttpResponse.json({ status: 'accepted' }, { status: 202 })
    }),
  )
  const { screen } = await renderWithRouter(<ConfirmSubscription token="tok-1" />)

  await expect.element(screen.getByText('Link expired')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('button', { name: 'Confirm subscription' }))
    .not.toBeInTheDocument()
  await screen.getByRole('button', { name: 'Send a new link' }).click()

  await expect.element(screen.getByText('Check your inbox')).toBeInTheDocument()
  expect(calls).toEqual(['renewed'])
})

test('a page without a token says the link is not valid', async () => {
  const { screen } = await renderWithRouter(<ConfirmSubscription />)

  await expect.element(screen.getByText('Link not valid')).toBeInTheDocument()
})

test('a token the server refuses says the link is not valid', async () => {
  worker.use(handleSitePublicConfirmationsRead(() => problem(400, 'Bad Request')))
  const { screen } = await renderWithRouter(<ConfirmSubscription token="garbage" />)

  await expect.element(screen.getByText('Link not valid')).toBeInTheDocument()
})
