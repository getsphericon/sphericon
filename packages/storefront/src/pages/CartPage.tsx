import { Button, Group, Stack, Text, Title } from '@mantine/core'

import { useCart } from '../cart.tsx'

export function CartPage() {
  const cart = useCart()
  const total = cart.items.reduce((sum, item) => sum + item.price, 0)

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
        </Stack>
      )}
    </>
  )
}
