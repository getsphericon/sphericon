import { Alert, Title } from '@mantine/core'
import { useState } from 'react'

import { EmailForm } from '../EmailForm.tsx'
import { signUp } from '../tracking.ts'

export function SignupPage() {
  const [welcomed, setWelcomed] = useState<string | null>(null)

  const submit = async (email: string) => {
    await signUp(email)
    setWelcomed(email)
  }

  return (
    <>
      <Title order={1} mb="md">
        Create your account
      </Title>
      {welcomed === null ? (
        <EmailForm submitLabel="Sign up" onSubmit={submit} />
      ) : (
        <Alert color="green">Welcome to Brightpath, {welcomed}.</Alert>
      )}
    </>
  )
}
