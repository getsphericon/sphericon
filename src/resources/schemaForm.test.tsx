import { Button, Checkbox, Select, TextInput } from '@mantine/core'
import { useState } from 'react'
import { expect, test } from 'vitest'
import { cdp, userEvent } from 'vitest/browser'
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
      <Button type="submit">{submitLabel}</Button>
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
  // Leaving Plan for a Name that is valid: an empty one would show its own error on the next blur,
  // which moves Plan down between the press and the release of the click that opens it.
  await name.fill('B')
  await expect.element(screen.getByText('Choose a plan.')).toBeInTheDocument()

  await plan.click()
  await screen.getByRole('option', { name: 'Free' }).click()
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

// press is a mouse press on a point of the page: the button goes down, the page may move under the
// pointer (a blur shows an error and the layout shifts), `whileDown` runs with the button held, and
// the button comes up where it went down, or at `releaseAt`. A click that lands on an element in one
// piece (locator.click) cannot show that.
async function press(
  x: number,
  y: number,
  {
    releaseAt = { x, y },
    whileDown,
  }: { releaseAt?: { x: number; y: number }; whileDown?: () => Promise<void> } = {},
) {
  const frame = window.frameElement?.getBoundingClientRect()
  const scale = frame ? frame.width / window.innerWidth : 1
  const at = (point: { x: number; y: number }) => ({
    x: (frame?.x ?? 0) + point.x * scale,
    y: (frame?.y ?? 0) + point.y * scale,
    button: 'left' as const,
    clickCount: 1,
  })
  const session = cdp()
  await session.send('Input.dispatchMouseEvent', { ...at({ x, y }), type: 'mouseMoved' })
  await session.send('Input.dispatchMouseEvent', { ...at({ x, y }), type: 'mousePressed' })
  await whileDown?.()
  await session.send('Input.dispatchMouseEvent', { ...at(releaseAt), type: 'mouseMoved' })
  await session.send('Input.dispatchMouseEvent', { ...at(releaseAt), type: 'mouseReleased' })
}

function topEdgeOf(button: Element) {
  const { left, top, width } = button.getBoundingClientRect()
  return { x: left + width / 2, y: top + 2 }
}

const emailError = 'Enter a valid email address.'
const otherLabel = 'Save other'
const pixel = 'data:image/gif;base64,R0lGODlhAQABAAAAACw='

// An image input submits its form as a submit button does.
function ImageSubmitForm() {
  const form = useSchemaForm(schema, { name: '', email: '', plan: '', agree: false })
  return (
    <form onSubmit={form.onSubmit(() => undefined)}>
      <TextInput label="Name" {...form.getInputProps('name')} />
      <TextInput label="Email" {...form.getInputProps('email')} />
      <input type="image" alt={submitLabel} src={pixel} width={80} height={36} />
    </form>
  )
}

// The blur's error moves the submit button down, so the top edge the press started on is gone by
// the time the mouse comes up: the press there is the one a click loses.
test.each([
  ['a submit button', Form],
  ['an image input', ImageSubmitForm],
])(
  'a press on the top edge of %s submits when the blur it causes moves the button',
  async (_, Component) => {
    const screen = await renderWithProviders(<Component />)
    await screen.getByLabelText('Email').click()
    const { x, y } = topEdgeOf(screen.getByRole('button', { name: submitLabel }).element())

    await press(x, y)

    await expect.element(screen.getByText(emailError)).toBeInTheDocument()
    await expect.element(screen.getByText('Name is required.')).toBeInTheDocument()
  },
)

test('a press that is dragged off the submit button still checks the field it left', async () => {
  const { screen, email, nameError } = await open()
  await email.click()
  const { x, y } = topEdgeOf(screen.getByRole('button', { name: submitLabel }).element())

  await press(x, y, { releaseAt: { x, y: y - 60 } })

  await expect.element(screen.getByText(emailError)).toBeInTheDocument()
  await expect.element(nameError).not.toBeInTheDocument()
})

test('a press the browser cancels checks the field it left without waiting for the button to come up', async () => {
  const { screen, email } = await open()
  await email.click()
  const button = screen.getByRole('button', { name: submitLabel }).element()
  const { x, y } = topEdgeOf(button)

  await press(x, y, {
    releaseAt: { x, y: y - 60 },
    whileDown: async () => {
      button.dispatchEvent(new PointerEvent('pointercancel', { bubbles: true, isPrimary: true }))
      await expect.element(screen.getByText(emailError)).toBeInTheDocument()
    },
  })
})

// Only a press on the field's own form waits: another form's submit button does not submit it.
function TwoSubmitForms() {
  const other = useSchemaForm(schema, { name: '', email: '', plan: '', agree: false })
  const form = useSchemaForm(schema, { name: '', email: '', plan: '', agree: false })
  return (
    <>
      <form onSubmit={other.onSubmit(() => undefined)}>
        <Button type="submit">{otherLabel}</Button>
      </form>
      <form onSubmit={form.onSubmit(() => undefined)}>
        <TextInput label="Email" {...form.getInputProps('email')} />
        <Button type="submit">{submitLabel}</Button>
      </form>
    </>
  )
}

test('a field left for another form’s submit button is checked when it loses focus', async () => {
  const screen = await renderWithProviders(<TwoSubmitForms />)
  await screen.getByLabelText('Email').click()
  const { x, y } = topEdgeOf(screen.getByRole('button', { name: otherLabel }).element())

  await press(x, y, {
    whileDown: async () => {
      await expect.element(screen.getByText(emailError)).toBeInTheDocument()
    },
  })
})

// A form that checks a field by a rule of its own sets that error on submit (ProviderLimitForm):
// the check of the field the press left must not run after the submit and clear it.
function OwnRuleForm() {
  const form = useSchemaForm(z.object({ maxPerDay: z.string() }), { maxPerDay: '' })
  return (
    <form
      onSubmit={form.onSubmit(({ maxPerDay }) => {
        if (!maxPerDay) {
          form.setErrors({ maxPerDay: 'Max per day is required.' })
        }
      })}
    >
      <TextInput label="Max per day" {...form.getInputProps('maxPerDay')} />
      <Button type="submit">{submitLabel}</Button>
    </form>
  )
}

test('an error the form sets on submit stays after a press that left a field', async () => {
  const screen = await renderWithProviders(<OwnRuleForm />)
  await screen.getByLabelText('Max per day').click()
  const { x, y } = topEdgeOf(screen.getByRole('button', { name: submitLabel }).element())

  await press(x, y)
  await new Promise<void>((resolve) => {
    setTimeout(resolve, 50)
  })

  await expect.element(screen.getByText('Max per day is required.')).toBeInTheDocument()
})

// The browser's own validation stops a submit on the first required field, before form.onSubmit
// runs, so the field the press left must show its error without the submit.
function NativeForm() {
  const form = useSchemaForm(schema, { name: '', email: '', plan: '', agree: false })
  return (
    <form onSubmit={form.onSubmit(() => undefined)}>
      <TextInput label="Name" required {...form.getInputProps('name')} />
      <TextInput label="Email" type="email" required {...form.getInputProps('email')} />
      <Button type="submit">{submitLabel}</Button>
    </form>
  )
}

test('a field left for the submit button shows its error when the browser stops the submit', async () => {
  const screen = await renderWithProviders(<NativeForm />)
  await screen.getByLabelText(/^Email/).click()
  const { x, y } = topEdgeOf(screen.getByRole('button', { name: submitLabel }).element())

  await press(x, y)

  await expect.element(screen.getByText(emailError)).toBeInTheDocument()
  await expect.element(screen.getByText('Name is required.')).not.toBeInTheDocument()
})
