import { Checkbox, Select, TextInput } from '@mantine/core'
import { useState } from 'react'
import { expect, test } from 'vitest'
import { userEvent } from 'vitest/browser'
import * as z from 'zod'

import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { useSchemaForm } from './schemaForm.ts'

const schema = z.object({
  name: z.string().min(1, 'Name is required.'),
  email: z.string().min(1, 'Enter a valid email address.'),
  plan: z.string().min(1, 'Choose a plan.'),
  agree: z.boolean().refine(Boolean, 'Accept the terms.'),
})

const submitLabel = 'Save'
const plans = ['Free', 'Pro']

function Form() {
  const form = useSchemaForm(schema, { name: '', email: '', plan: '', agree: false })
  return (
    <form onSubmit={form.onSubmit(() => undefined)}>
      <TextInput label="Name" {...form.getInputProps('name')} />
      <TextInput label="Email" {...form.getInputProps('email')} />
      <Select label="Plan" data={plans} {...form.getInputProps('plan')} />
      <Checkbox label="Agree" {...form.getInputProps('agree', { type: 'checkbox' })} />
      <button type="submit">{submitLabel}</button>
    </form>
  )
}

async function open() {
  const screen = await renderWithProviders(<Form />)
  return {
    screen,
    name: screen.getByLabelText('Name'),
    email: screen.getByLabelText('Email'),
    plan: screen.getByRole('combobox', { name: 'Plan' }),
    agree: screen.getByLabelText('Agree'),
    nameError: screen.getByText('Name is required.'),
  }
}

test('a field shows no error on open, shows one after blur and clears it on a valid keystroke', async () => {
  const { screen, name, email, nameError } = await open()
  await expect.element(nameError).not.toBeInTheDocument()
  await expect.element(screen.getByText('Enter a valid email address.')).not.toBeInTheDocument()

  await name.click()
  await email.click()
  await expect.element(nameError).toBeInTheDocument()

  await name.fill('Ada')
  await expect.element(nameError).not.toBeInTheDocument()
})

test('a valid field shows no error on blur', async () => {
  const { name, email, nameError } = await open()
  await name.fill('A')
  await email.click()
  await expect.element(nameError).not.toBeInTheDocument()
})

test('an error that shows is re-checked on each keystroke, a cleared one waits for the next blur', async () => {
  const { name, email, nameError } = await open()
  await name.click()
  await email.click()
  await expect.element(nameError).toBeInTheDocument()

  await name.fill('B')
  await expect.element(nameError).not.toBeInTheDocument()
  await name.fill('')
  await expect.element(nameError).not.toBeInTheDocument()
  await email.click()
  await expect.element(nameError).toBeInTheDocument()
})

test('a Select shows its error after blur and clears it when an option is chosen', async () => {
  const { screen, plan, name } = await open()
  await plan.click()
  await name.click()
  await expect.element(screen.getByText('Choose a plan.')).toBeInTheDocument()

  await plan.click()
  await userEvent.keyboard('{ArrowDown}{Enter}')
  await expect.element(plan).toHaveValue('Free')
  await expect.element(screen.getByText('Choose a plan.')).not.toBeInTheDocument()
})

test('a Checkbox is checked again when its error shows and is checked', async () => {
  const { screen, agree } = await open()
  await screen.getByRole('button', { name: submitLabel }).click()
  await expect.element(screen.getByText('Accept the terms.')).toBeInTheDocument()

  await agree.click()
  await expect.element(screen.getByText('Accept the terms.')).not.toBeInTheDocument()
})

test('a failed submit focuses the first invalid field in the page order', async () => {
  const { screen, name, email } = await open()
  await name.fill('Ada')
  await screen.getByRole('button', { name: submitLabel }).click()

  await expect.element(screen.getByText('Enter a valid email address.')).toBeInTheDocument()
  await expect.element(email).toHaveFocus()

  await name.fill('')
  await userEvent.keyboard('{Enter}')
  await expect.element(name).toHaveFocus()
})

const refuseLabel = 'Refuse'

// ServerForm hands a server's refusal to the form and writes down what came back.
function ServerForm({ refusal }: { refusal: unknown }) {
  const form = useSchemaForm(schema, { name: '', email: '', plan: '', agree: false })
  const [handedBack, setHandedBack] = useState<string>()
  return (
    <form onSubmit={form.onSubmit(() => undefined)}>
      <TextInput label="Name" {...form.getInputProps('name')} />
      <TextInput label="Email" {...form.getInputProps('email')} />
      <Checkbox label="Agree" {...form.getInputProps('agree', { type: 'checkbox' })} />
      <button
        type="button"
        onClick={() => setHandedBack(JSON.stringify(form.showFieldErrors(refusal) ?? null))}
      >
        {refuseLabel}
      </button>
      <output>{handedBack}</output>
    </form>
  )
}

async function refuse(refusal: unknown) {
  const screen = await renderWithProviders(<ServerForm refusal={refusal} />)
  await screen.getByRole('button', { name: refuseLabel }).click()
  return screen
}

test('a refusal from the server shows under its fields and focuses the first in the page order', async () => {
  const screen = await refuse({
    status: 422,
    errors: { agree: ['Accept the terms first.'], email: ['That address is taken.'] },
  })

  await expect.element(screen.getByText('That address is taken.')).toBeInTheDocument()
  await expect.element(screen.getByText('Accept the terms first.')).toBeInTheDocument()
  await expect.element(screen.getByLabelText('Email')).toHaveFocus()
  await expect.element(screen.getByRole('status')).toHaveTextContent('null')
})

test('a refusal of a field the form does not render is handed back, the rest shows', async () => {
  const screen = await refuse({
    status: 422,
    errors: { name: ['Name is too long.'], token: ['The link expired.'] },
  })

  await expect.element(screen.getByText('Name is too long.')).toBeInTheDocument()
  await expect.element(screen.getByLabelText('Name')).toHaveFocus()
  await expect
    .element(screen.getByRole('status'))
    .toHaveTextContent(JSON.stringify({ token: ['The link expired.'] }))
})

test('a refusal that names no field is handed back whole', async () => {
  const screen = await refuse({ status: 403, title: 'Forbidden' })

  await expect.element(screen.getByRole('status')).toHaveTextContent('{}')
  await expect.element(screen.getByLabelText('Name')).not.toHaveFocus()
})

const otherSchema = z.object({ password: z.string() })

// TwoForms puts a form with no password field beside one that has it.
function TwoForms({ refusal }: { refusal: unknown }) {
  const form = useSchemaForm(schema, { name: '', email: '', plan: '', agree: false })
  const other = useSchemaForm(otherSchema, { password: '' })
  const [handedBack, setHandedBack] = useState<string>()
  return (
    <>
      <form onSubmit={form.onSubmit(() => undefined)}>
        <TextInput label="Name" {...form.getInputProps('name')} />
        <button
          type="button"
          onClick={() => setHandedBack(JSON.stringify(form.showFieldErrors(refusal) ?? null))}
        >
          {refuseLabel}
        </button>
        <output>{handedBack}</output>
      </form>
      <form onSubmit={other.onSubmit(() => undefined)}>
        <TextInput label="Password" {...other.getInputProps('password')} />
      </form>
    </>
  )
}

test('a refusal of a field only another form renders is handed back, not placed there', async () => {
  const screen = await renderWithProviders(
    <TwoForms refusal={{ status: 422, errors: { password: ['Use at least 8 characters.'] } }} />,
  )
  await screen.getByRole('button', { name: refuseLabel }).click()

  await expect
    .element(screen.getByRole('status'))
    .toHaveTextContent(JSON.stringify({ password: ['Use at least 8 characters.'] }))
  await expect.element(screen.getByLabelText('Password')).not.toHaveFocus()
})
