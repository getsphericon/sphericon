import { TextInput } from '@mantine/core'
import { expect, test } from 'vitest'
import { userEvent } from 'vitest/browser'
import * as z from 'zod'

import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { useSchemaForm } from './schemaForm.ts'

const schema = z.object({
  name: z.string().min(1, 'Name is required.'),
  email: z.string().min(1, 'Enter a valid email address.'),
})

function Form() {
  const form = useSchemaForm(schema, { name: '', email: '' })
  return (
    <form onSubmit={form.onSubmit(() => undefined)}>
      <TextInput label="Name" {...form.getInputProps('name')} />
      <TextInput label="Email" {...form.getInputProps('email')} />
      <button type="submit">{'Save'}</button>
    </form>
  )
}

test('a field shows no error on open, shows one after blur and clears it on a valid keystroke', async () => {
  const screen = await renderWithProviders(<Form />)
  await expect.element(screen.getByText('Name is required.')).not.toBeInTheDocument()

  const name = screen.getByLabelText('Name')
  await name.click()
  await screen.getByLabelText('Email').click()
  await expect.element(screen.getByText('Name is required.')).toBeInTheDocument()

  await name.fill('Ada')
  await expect.element(screen.getByText('Name is required.')).not.toBeInTheDocument()
})

test('an error that shows is re-checked on each keystroke', async () => {
  const screen = await renderWithProviders(<Form />)
  const name = screen.getByLabelText('Name')
  await name.fill('A')
  await screen.getByLabelText('Email').click()
  await expect.element(screen.getByText('Name is required.')).not.toBeInTheDocument()

  await name.fill('')
  await screen.getByLabelText('Email').click()
  await expect.element(screen.getByText('Name is required.')).toBeInTheDocument()

  await name.fill('B')
  await expect.element(screen.getByText('Name is required.')).not.toBeInTheDocument()
  await name.fill('')
  await expect.element(screen.getByText('Name is required.')).not.toBeInTheDocument()
  await screen.getByLabelText('Email').click()
  await expect.element(screen.getByText('Name is required.')).toBeInTheDocument()
})

test('a failed submit focuses the first invalid field in the page order', async () => {
  const screen = await renderWithProviders(<Form />)
  await screen.getByLabelText('Name').fill('Ada')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('Enter a valid email address.')).toBeInTheDocument()
  await expect.element(screen.getByLabelText('Email')).toHaveFocus()

  await screen.getByLabelText('Name').fill('')
  await userEvent.keyboard('{Enter}')
  await expect.element(screen.getByLabelText('Name')).toHaveFocus()
})
