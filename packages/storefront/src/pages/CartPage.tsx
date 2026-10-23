import { Alert, Button, Group, Stack, Text, TextInput, Title } from '@mantine/core'
import { useState } from 'react'
import type { SubmitEvent } from 'react'

import { useCart } from '../cart.tsx'
import { enrolled } from '../tracking.ts'

export function CartPage() {
  const cart = useCart()
  const [email, setEmail] = useState('')
  const [status, setStatus] = useState<'idle' | 'done' | 'failed'>('idle')
  const total = cart.items.reduce((sum, item) => sum + item.price, 0)

  const enroll = async (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault()
    try {
      await enrolled(
        email,
        cart.items.map((item) => item.slug),
      )
      setStatus('done')
    } catch {
      setStatus('failed')
    }
  }

  return (
    <>
      <Title order={1} mb="md">
        Your cart
      </Title>
      {cart.items.length === 0 ? (
        <Text c="dimmed">Your cart is empty.</Text>
      ) : (
        <Stack>
          {cart.items.map((item) => (
            <Group key={item.slug} justify="space-between">
              <Text>{item.title}</Text>
              <Group>
                <Text fw={700}>${item.price}</Text>
                <Button
                  size="xs"
                  variant="subtle"
                  color="red"
                  onClick={() => {
                    cart.remove(item.slug)
                  }}
                >
                  Remove
                </Button>
              </Group>
            </Group>
          ))}
          <Group justify="space-between">
            <Text fw={700}>Total</Text>
            <Text fw={700}>${total}</Text>
          </Group>
          <form onSubmit={(event) => void enroll(event)}>
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
              <Button type="submit">Enroll</Button>
              {status === 'done' && <Alert color="green">You are enrolled.</Alert>}
              {status === 'failed' && <Alert color="red">Enrolling failed, try again.</Alert>}
            </Stack>
          </form>
        </Stack>
      )}
    </>
  )
}
