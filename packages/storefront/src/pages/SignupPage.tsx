import { Alert, Button, Stack, TextInput, Title } from '@mantine/core'
import { useState } from 'react'
import type { SubmitEvent } from 'react'

import { signedUp } from '../tracking.ts'

export function SignupPage() {
  const [email, setEmail] = useState('')
  const [done, setDone] = useState(false)

  const submit = async (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault()
    await signedUp(email)
    setDone(true)
  }

  return (
    <>
      <Title order={1} mb="md">
        Create your account
      </Title>
      {done ? (
        <Alert color="green">Welcome to Brightpath, {email}.</Alert>
      ) : (
        <form onSubmit={(event) => void submit(event)}>
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
            <Button type="submit">Sign up</Button>
          </Stack>
        </form>
      )}
    </>
  )
}
