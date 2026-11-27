import { Card, Loader, Stack, Text, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import type { QueryKey } from '@tanstack/react-query'

import { Route as loginRoute } from '../routes/login.tsx'
import { oneShotTokenQuery } from '../utils/oneShotToken.ts'
import { CenteredLayout } from './CenteredLayout.tsx'
import { QueryBoundary } from './QueryBoundary.tsx'
import { AnchorLink } from './RouterLink.tsx'

// What a one-shot link page says while it works, once it is done and when the link is refused.
interface OneShotLinkCopy {
  working: string
  successTitle: string
  successBody: string
  goToLogin: string
  errorTitle: string
  errorBody: string
}

interface OneShotLinkProps {
  // The mutation key of the operation the link's token is sent to, which names the token.
  queryKey: QueryKey
  // Sends the token, throwing on a refusal.
  send: () => Promise<unknown>
  copy: OneShotLinkCopy
}

// A refused link (400) is spent, garbage or expired, an answer rather than a fault; a failure of
// the request itself ends on the same words.
function Refused({ copy }: { copy: OneShotLinkCopy }) {
  return (
    <>
      <Title order={3}>{copy.errorTitle}</Title>
      <Text c="dimmed" ta="center">
        {copy.errorBody}
      </Text>
    </>
  )
}

// Opening the link is the action: the page reads what its token yields, once.
function Answer({ queryKey, send, copy }: OneShotLinkProps) {
  const { data: accepted } = useSuspenseQuery(
    oneShotTokenQuery(
      queryKey,
      async () => {
        await send()
        return true
      },
      false,
    ),
  )

  if (!accepted) {
    return <Refused copy={copy} />
  }

  return (
    <>
      <Title order={3}>{copy.successTitle}</Title>
      <Text c="dimmed" ta="center">
        {copy.successBody}
      </Text>
      <AnchorLink to={loginRoute.to} size="sm" mt="md">
        {copy.goToLogin}
      </AnchorLink>
    </>
  )
}

// OneShotLinkPage is where a mailed link that acts once lands (confirming an email change,
// reporting a sign-in that was not the User's): it sends the token and says how that went.
export function OneShotLinkPage(props: OneShotLinkProps) {
  const { copy } = props
  return (
    <CenteredLayout maw={460} align="center">
      <Card withBorder w="100%" padding="xl">
        <Stack align="center" gap="sm">
          <QueryBoundary
            fallback={
              <>
                <Loader />
                <Text c="dimmed">{copy.working}</Text>
              </>
            }
            errorTitle={copy.errorTitle}
            renderError={() => <Refused copy={copy} />}
          >
            <Answer {...props} />
          </QueryBoundary>
        </Stack>
      </Card>
    </CenteredLayout>
  )
}
