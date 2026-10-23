import { Button, Stack, TextInput } from '@mantine/core'
import { useState } from 'react'
import type { ReactNode } from 'react'

interface EmailFormProps {
  submitLabel: string
  onSubmit: (email: string) => Promise<void>
  // Shown under the button: the outcome of the submit.
  children?: ReactNode
}

// The email form of the signup and the cart: one field and one button.
export function EmailForm({ submitLabel, onSubmit, children }: EmailFormProps) {
  const [email, setEmail] = useState('')

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        void onSubmit(email)
      }}
    >
      <Stack maw={400}>
        <TextInput
          label="Email"
          type="email"
          required
          value={email}
          onChange={(event) => {
            setEmail(event.currentTarget.value)
          }}
        />
        <Button type="submit">{submitLabel}</Button>
        {children}
      </Stack>
    </form>
  )
}
