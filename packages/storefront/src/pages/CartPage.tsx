import { Alert, Button, Group, Stack, Text, Title } from '@mantine/core'
import { useState } from 'react'

import { useCart } from '../cartContext.ts'
import { totalPrice } from '../courses.ts'
import { EmailForm } from '../EmailForm.tsx'
import { enroll } from '../tracking.ts'

export function CartPage() {
  const cart = useCart()
  const [status, setStatus] = useState<'idle' | 'done' | 'failed'>('idle')
  const total = totalPrice(cart.items)

  const submit = async (email: string) => {
    try {
      await enroll(
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
          <EmailForm submitLabel="Enroll" onSubmit={submit}>
            {status === 'done' && <Alert color="green">You are enrolled.</Alert>}
            {status === 'failed' && <Alert color="red">Enrolling failed, try again.</Alert>}
          </EmailForm>
        </Stack>
      )}
    </>
  )
}
