import { expect, test } from 'vitest'

import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { EventActionName, EventActionOption } from './EventActionName.tsx'

// Every System event, and the audit entry a webhook subscribes to, has its name on screen.
const names: [string, string][] = [
  ['contact.created', 'Became a contact'],
  ['contact.erased', 'Was erased'],
  ['email.sent', 'Was sent an email'],
  ['email.delivered', 'Received an email'],
  ['email.delayed', 'Email delayed'],
  ['email.dropped', 'Email dropped'],
  ['email.bounced', 'Bounced'],
  ['email.complained', 'Marked an email as spam'],
  ['email.opened', 'Opened an email'],
  ['email.clicked', 'Clicked a link in an email'],
  ['email.unsubscribed', 'Unsubscribed'],
  ['marketing.confirmed', 'Confirmed opt-in'],
  ['audit.entry', 'Audit log entry'],
]

test.each(names)('%s is shown as "%s"', async (action, name) => {
  const screen = await renderWithProviders(<EventActionName action={action} />)

  await expect.element(screen.getByText(name, { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText(action)).not.toBeInTheDocument()
})

test("a customer's action is shown as it was tracked", async () => {
  const screen = await renderWithProviders(<EventActionName action="Order Completed" />)

  await expect.element(screen.getByText('Order Completed')).toBeInTheDocument()
})

test('a System event keeps its action on hover', async () => {
  const screen = await renderWithProviders(<EventActionName action="email.opened" />)

  await screen.getByText('Opened an email').hover()

  await expect.element(screen.getByText('email.opened')).toBeInTheDocument()
})

test('an option of a System event names it and shows the action it puts in the field', async () => {
  const screen = await renderWithProviders(<EventActionOption action="email.clicked" />)

  await expect.element(screen.getByText('Clicked a link in an email')).toBeInTheDocument()
  await expect.element(screen.getByText('email.clicked')).toBeInTheDocument()
})

test("an option of a customer's action is the action alone", async () => {
  const screen = await renderWithProviders(<EventActionOption action="purchase" />)

  await expect.element(screen.getByText('purchase')).toBeInTheDocument()
})
