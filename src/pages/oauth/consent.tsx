import {
  Alert,
  Badge,
  Button,
  Checkbox,
  Group,
  Loader,
  Paper,
  Select,
  Stack,
  Text,
  Title,
} from '@mantine/core'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  siteOAuthDecideMutation,
  siteOAuthDescribeOptions,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as oauthConsentRoute } from '../../routes/oauth/consent.tsx'
import { getApiErrorMessage, isForbiddenError } from '../../utils/apiErrors.ts'

// The authorization request exactly as the OAuth client sent it (the backend's
// /oauth/authorize hands the browser here with the same query string).
export interface ConsentRequest {
  clientId: string
  redirectUri: string
  codeChallenge: string
  state: string
  scope: string
}

export function OAuthConsentPage() {
  const search = oauthConsentRoute.useSearch()
  return (
    <OAuthConsent
      request={{
        clientId: search.client_id,
        redirectUri: search.redirect_uri,
        codeChallenge: search.code_challenge,
        state: search.state,
        scope: search.scope,
      }}
    />
  )
}

// OAuthConsent asks the signed-in user whether an MCP client (claude.ai
// connector, ...) may act on one of their workspaces. Approving makes the backend
// mint a one-time code; the browser is then sent back to the client.
export function OAuthConsent({ request }: { request: ConsentRequest }) {
  const { t } = useTranslation()
  const [workspaceId, setWorkspaceId] = useState<string | null>(null)
  const [allowSend, setAllowSend] = useState(false)

  const describe = useQuery({
    ...siteOAuthDescribeOptions({
      query: {
        clientId: request.clientId,
        redirectUri: request.redirectUri,
        scope: request.scope,
      },
    }),
    retry: false,
  })
  const workspaces = useQuery(siteWorkspacesListOptions())

  const decide = useMutation({
    ...siteOAuthDecideMutation(),
    // Hand the browser back to the client (an external URL the server built).
    onSuccess: (data) => window.location.assign(data.redirectUrl),
    onError: (error) => {
      // A 403 is explained inline (below) so it stays visible; other failures toast.
      if (isForbiddenError(error)) {
        return
      }
      notifications.show({
        color: 'red',
        title: t(($) => $.oauthConsent.errorTitle),
        message: getApiErrorMessage(
          error,
          t(($) => $.oauthConsent.errorMessage),
        ),
      })
    },
  })

  if (describe.isLoading || workspaces.isLoading) {
    return (
      <Stack maw={480} mx="auto" mt="xl" align="center">
        <Loader />
      </Stack>
    )
  }

  if (describe.isError || !describe.data) {
    return (
      <Stack maw={480} mx="auto" mt="xl">
        <Alert color="red" title={t(($) => $.oauthConsent.errorTitle)}>
          {t(($) => $.oauthConsent.invalid)}
        </Alert>
      </Stack>
    )
  }

  const info = describe.data
  const options = (workspaces.data ?? []).map((w) => ({ value: w.id, label: w.name }))
  const selected = workspaceId ?? options[0]?.value ?? null

  const submit = (approve: boolean) => {
    if (!selected) {
      return
    }
    decide.mutate({
      body: {
        clientId: request.clientId,
        redirectUri: request.redirectUri,
        codeChallenge: request.codeChallenge,
        state: request.state,
        scope: request.scope,
        workspaceId: selected,
        approve,
        allowSend: approve && allowSend,
      },
    })
  }

  return (
    <Stack maw={480} mx="auto" mt="xl">
      <Title order={3}>{t(($) => $.oauthConsent.title, { client: info.clientName })}</Title>
      <Text>{t(($) => $.oauthConsent.intro, { client: info.clientName })}</Text>
      {info.clientHost ? (
        <Text size="sm" c="dimmed">
          {t(($) => $.oauthConsent.clientHost, { host: info.clientHost })}
        </Text>
      ) : null}

      {info.localRedirect ? (
        <Alert color="yellow" title={t(($) => $.oauthConsent.localhostTitle)}>
          {t(($) => $.oauthConsent.localhostDescription)}
        </Alert>
      ) : null}

      <Select
        label={t(($) => $.oauthConsent.workspaceLabel)}
        data={options}
        value={selected}
        onChange={setWorkspaceId}
        allowDeselect={false}
      />

      <Paper withBorder p="md">
        <Stack gap="xs">
          <Text fw={500}>{t(($) => $.oauthConsent.scopesTitle)}</Text>
          <Group gap="xs">
            {info.scopes.map((scope) => (
              <Badge key={scope} variant="light">
                {scope}
              </Badge>
            ))}
          </Group>
        </Stack>
      </Paper>

      {info.sendScopes.length > 0 ? (
        <Alert color="yellow" title={t(($) => $.oauthConsent.sendTitle)}>
          <Stack gap="xs">
            <Text size="sm">{t(($) => $.oauthConsent.sendDescription)}</Text>
            <Checkbox
              checked={allowSend}
              onChange={(event) => setAllowSend(event.currentTarget.checked)}
              label={t(($) => $.oauthConsent.sendCheckbox, { scopes: info.sendScopes.join(', ') })}
            />
          </Stack>
        </Alert>
      ) : null}

      {decide.isError && isForbiddenError(decide.error) ? (
        <Alert color="red" title={t(($) => $.oauthConsent.errorTitle)}>
          {t(($) => $.oauthConsent.forbidden)}
        </Alert>
      ) : null}

      <Group justify="flex-end">
        <Button variant="default" onClick={() => submit(false)} disabled={decide.isPending}>
          {t(($) => $.oauthConsent.deny)}
        </Button>
        <Button onClick={() => submit(true)} loading={decide.isPending} disabled={!selected}>
          {t(($) => $.oauthConsent.allow)}
        </Button>
      </Group>
    </Stack>
  )
}
