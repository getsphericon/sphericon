import { Center, Container, Stack } from '@mantine/core'
import type { ReactNode } from 'react'

// CenteredLayout puts a standalone page (sign-in, confirmation, consent) in a Container, whose
// side padding keeps the content off the edges of a phone, and centres the content inside it.
export function CenteredLayout({
  maw = 400,
  align,
  children,
}: {
  maw?: number
  align?: 'center'
  children: ReactNode
}) {
  return (
    <Container size={maw} w="100%">
      <Center mih="100dvh" py="xl">
        <Stack w="100%" align={align}>
          {children}
        </Stack>
      </Center>
    </Container>
  )
}
