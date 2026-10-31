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
import { useMutation, useSuspenseQueries } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../../components/CenteredLayout.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import {
  siteOAuthDecideMutation,
  siteOAuthDescribeOptions,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { Route as oauthConsentRoute } from '../../routes/oauth/consent.tsx'
import { getApiErrorMessage, isForbiddenError } from '../../utils/apiErrors.ts'
import { can } from '../../utils/permissions.ts'

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
// mint a one-time code; the browser is then sent back to the client. Any member may
// approve, but only the scopes their Role may grant in the chosen workspace (ADR 0061):
// the screen offers no other and sends only those, and the server refuses the rest.
export function OAuthConsent({ request }: { request: ConsentRequest }) {
  const { t } = useTranslation()
  return (
    <QueryBoundary
      fallback={
        <CenteredLayout maw={480} align="center">
          <Loader />
        </CenteredLayout>
      }
      errorTitle={t(($) => $.oauthConsent.errorTitle)}
      // A request the server will not describe cannot be granted: it is shown as invalid.
      renderError={() => (
        <CenteredLayout maw={480}>
          <Alert color="red" title={t(($) => $.oauthConsent.errorTitle)}>
            {t(($) => $.oauthConsent.invalid)}
          </Alert>
        </CenteredLayout>
      )}
    >
      <ConsentForm request={request} />
    </QueryBoundary>
  )
}

function ConsentForm({ request }: { request: ConsentRequest }) {
  const { t } = useTranslation()
  const notify = useNotify()
  const [workspaceId, setWorkspaceId] = useState<string | null>(null)
  const [allowSend, setAllowSend] = useState(false)
  const [allowSetup, setAllowSetup] = useState(false)

  const [{ data: info }, { data: workspaces }] = useSuspenseQueries({
    queries: [
      {
        ...siteOAuthDescribeOptions({
          query: {
            clientId: request.clientId,
            redirectUri: request.redirectUri,
            scope: request.scope,
          },
        }),
        retry: false,
      },
      siteWorkspacesListOptions(),
    ],
  })

  const decide = useMutation({
    ...siteOAuthDecideMutation(),
    // Hand the browser back to the client (an external URL the server built).
    onSuccess: (data) => window.location.assign(data.redirectUrl),
    onError: (error) => {
      // A 403 is explained inline (below) so it stays visible; other failures toast.
      if (isForbiddenError(error)) {
        return
      }
      notify({
        color: 'red',
        title: t(($) => $.oauthConsent.errorTitle),
        message: getApiErrorMessage(
          error,
          t(($) => $.oauthConsent.errorMessage),
        ),
      })
    },
  })

  const options = workspaces.map((w) => ({ value: w.id, label: w.name }))
  const selected = workspaceId ?? options[0]?.value ?? null
  const workspace = workspaces.find((w) => w.id === selected)
  // A scope whose Permission the User's Role lacks in the chosen workspace is not offered.
  const grantable = <S extends string>(scopes: S[]) =>
    scopes.filter((scope) => {
      const needed = info.requiredPermissions[scope]
      return needed === undefined || can(workspace, needed)
    })
  const scopes = grantable(info.scopes)
  const sendScopes = grantable(info.sendScopes)
  const setupScopes = grantable(info.setupScopes)
  const offered = [...scopes, ...sendScopes, ...setupScopes]

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
        scope: offered.join(' '),
        workspaceId: selected,
        approve,
        allowSend: approve && allowSend,
        allowSetup: approve && allowSetup,
      },
    })
  }

  return (
    <CenteredLayout maw={480}>
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
            {scopes.map((scope) => (
              <Badge key={scope} variant="light">
                {scope}
              </Badge>
            ))}
          </Group>
        </Stack>
      </Paper>

      {sendScopes.length > 0 ? (
        <Alert color="yellow" title={t(($) => $.oauthConsent.sendTitle)}>
          <Stack gap="xs">
            <Text size="sm">{t(($) => $.oauthConsent.sendDescription)}</Text>
            <Checkbox
              checked={allowSend}
              onChange={(event) => setAllowSend(event.currentTarget.checked)}
              label={t(($) => $.oauthConsent.sendCheckbox, { scopes: sendScopes.join(', ') })}
            />
          </Stack>
        </Alert>
      ) : null}

      {setupScopes.length > 0 ? (
        <Alert color="yellow" title={t(($) => $.oauthConsent.setupTitle)}>
          <Stack gap="xs">
            <Text size="sm">{t(($) => $.oauthConsent.setupDescription)}</Text>
            <Checkbox
              checked={allowSetup}
              onChange={(event) => setAllowSetup(event.currentTarget.checked)}
              label={t(($) => $.oauthConsent.setupCheckbox, {
                scopes: setupScopes.join(', '),
              })}
            />
          </Stack>
        </Alert>
      ) : null}

      {workspace && offered.length === 0 ? (
        <Alert color="yellow">{t(($) => $.oauthConsent.nothingToGrant)}</Alert>
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
        <Button
          onClick={() => submit(true)}
          loading={decide.isPending}
          disabled={!selected || offered.length === 0}
        >
          {t(($) => $.oauthConsent.allow)}
        </Button>
      </Group>
    </CenteredLayout>
  )
}
