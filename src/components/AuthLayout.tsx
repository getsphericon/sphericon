import { Anchor, Box, Flex, Group, Image, List, Stack, Text, ThemeIcon, Title } from '@mantine/core'
import { IconCheck } from '@tabler/icons-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from './CenteredLayout.tsx'

// AuthLayout frames a sign-in style page (sign in, sign up, password recovery) with what the
// product is: a brand panel beside the form from the `md` breakpoint up, a compact brand header
// above it on a phone. Without it a visitor lands on a bare form and cannot tell which product it
// belongs to.
export function AuthLayout({ maw = 400, children }: { maw?: number; children: ReactNode }) {
  const { t } = useTranslation()
  const highlights = [
    t(($) => $.authLayout.highlightContacts),
    t(($) => $.authLayout.highlightAutomations),
    t(($) => $.authLayout.highlightEmail),
  ]

  return (
    <Flex mih="100dvh">
      <Stack
        visibleFrom="md"
        w="45%"
        maw={560}
        p="xl"
        justify="center"
        gap="lg"
        bg="var(--mantine-primary-color-light)"
      >
        <Group gap="sm" wrap="nowrap">
          <Image src="/favicon.svg" alt="" w={40} h={40} />
          <Title order={2}>{t(($) => $.authLayout.name)}</Title>
        </Group>
        <Title order={3} fw={500}>
          {t(($) => $.authLayout.tagline)}
        </Title>
        <List
          spacing="sm"
          icon={
            <ThemeIcon size={20} radius="xl">
              <IconCheck size={12} />
            </ThemeIcon>
          }
        >
          {highlights.map((highlight) => (
            <List.Item key={highlight}>{highlight}</List.Item>
          ))}
        </List>
        <Anchor href={t(($) => $.authLayout.websiteUrl)} target="_blank" rel="noreferrer" size="sm">
          {t(($) => $.authLayout.websiteLabel)}
        </Anchor>
      </Stack>
      <Box flex={1}>
        <CenteredLayout maw={maw}>
          <Stack hiddenFrom="md" gap="xs">
            <Group gap="sm" wrap="nowrap">
              <Image src="/favicon.svg" alt="" w={32} h={32} />
              <Title order={3}>{t(($) => $.authLayout.name)}</Title>
            </Group>
            <Text c="dimmed" size="sm">
              {t(($) => $.authLayout.tagline)}
            </Text>
          </Stack>
          {children}
        </CenteredLayout>
      </Box>
    </Flex>
  )
}
