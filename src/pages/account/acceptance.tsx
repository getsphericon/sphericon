import { Alert, Button, Card, Group, ScrollArea, Stack, Text, Title } from '@mantine/core'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { CenteredLayout } from '../../components/CenteredLayout.tsx'
import { DeleteWorkspaceControl } from '../../components/DeleteWorkspaceControl.tsx'
import { LegalMarkdown } from '../../components/LegalMarkdown.tsx'
import { BlockSkeleton } from '../../components/Skeletons.tsx'
import {
  siteLegalDocumentsGetOptions,
  siteUserAcceptPendingMutation,
  siteUserGetMeOptions,
  siteUserGetMeQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteSignupItem, SiteSoleOwnedWorkspace } from '../../generated/site/types.gen.ts'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { useLogout } from '../../hooks/useLogout.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as acceptanceRoute } from '../../routes/acceptance.tsx'
import { Route as indexRoute } from '../../routes/index.tsx'
import { DeleteUserSection } from './DeleteUserForm.tsx'

// PendingDocument is the current text of one required acceptance the User has not accepted.
function PendingDocument({ item }: { item: SiteSignupItem }) {
  const { formatDate } = useDateFormat()
  const { t } = useTranslation()
  const query = useQuery({
    ...siteLegalDocumentsGetOptions({ path: { key: item.key } }),
    retry: false,
  })

  return (
    <Stack gap="xs">
      <Title order={3}>{item.title}</Title>
      {query.isPending && <BlockSkeleton />}
      {query.isError && (
        <ApiErrorAlert
          error={query.error}
          title={t(($) => $.legal.loadError)}
          fallback={t(($) => $.notifications.errorMessage)}
        />
      )}
      {query.data && (
        <>
          <Text c="dimmed" size="sm">
            {t(($) => $.legal.version, {
              version: query.data.version,
              date: formatDate(query.data.publishedAt),
            })}
          </Text>
          <Card withBorder padding="lg">
            <ScrollArea.Autosize mah={360}>
              <LegalMarkdown markdown={query.data.markdown} />
            </ScrollArea.Autosize>
          </Card>
        </>
      )}
    </Stack>
  )
}

// SoleOwnedWorkspaces offers to delete each Workspace a refused account deletion named, the ones
// the User is the only Owner of (ADR 0050), so a gated User is not trapped: deleting a Workspace
// is answered while an acceptance is pending. Nothing is shown before such a refusal.
function SoleOwnedWorkspaces({
  workspaces,
  onDeleted,
}: {
  workspaces: SiteSoleOwnedWorkspace[]
  onDeleted: (id: string) => void
}) {
  const { t } = useTranslation()
  if (workspaces.length === 0) {
    return null
  }
  return (
    <Stack maw={420}>
      <Alert color="yellow">{t(($) => $.acceptance.soleOwned)}</Alert>
      {workspaces.map((workspace) => (
        <Stack key={workspace.id} gap="xs">
          <Text>{workspace.name}</Text>
          <DeleteWorkspaceControl
            workspace={workspace}
            label={t(($) => $.acceptance.deleteWorkspace, { name: workspace.name })}
            onDone={() => onDeleted(workspace.id)}
          />
        </Stack>
      ))}
    </Stack>
  )
}

// The accept-or-sign-out screen (ADR 0062): a required acceptance has a version the User has not
// accepted, so the app stops here before any other page. The User reads each text and accepts
// them all at the versions shown, signs out, or deletes their account, and first the Workspaces
// they are the only Owner of.
export function AcceptancePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  // The page the gate stopped the User on, resumed once they accept.
  const { redirect } = acceptanceRoute.useSearch()
  const meQuery = useQuery(siteUserGetMeOptions())
  const pending = meQuery.data?.acceptancesPending ?? []
  const [soleOwned, setSoleOwned] = useState<SiteSoleOwnedWorkspace[]>([])

  const accept = useResourceMutation({
    mutation: siteUserAcceptPendingMutation(),
    errorTitle: t(($) => $.acceptance.acceptErrorTitle),
    // A version published meanwhile refuses the acceptance: the screen reloads to show it.
    handleError: async () => {
      await queryClient.invalidateQueries({ queryKey: siteUserGetMeQueryKey() })
      return false
    },
    onDone: async (me) => {
      queryClient.setQueryData(siteUserGetMeQueryKey(), me)
      await navigate(redirect ? { href: redirect } : { to: indexRoute.to })
    },
  })

  const logout = useLogout()

  return (
    <CenteredLayout maw={720}>
      <Title order={1}>{t(($) => $.acceptance.title)}</Title>
      <Text>{t(($) => $.acceptance.intro)}</Text>
      {pending.map((item) => (
        <PendingDocument key={item.key} item={item} />
      ))}
      <Group justify="flex-end">
        <Button variant="default" loading={logout.isPending} onClick={() => logout.mutate({})}>
          {t(($) => $.acceptance.signOut)}
        </Button>
        <Button
          loading={accept.isPending}
          disabled={pending.length === 0}
          onClick={() =>
            accept.mutate({
              body: { items: pending.map(({ key, version }) => ({ key, version })) },
            })
          }
        >
          {t(($) => $.acceptance.accept)}
        </Button>
      </Group>
      <DeleteUserSection onSoleOwned={setSoleOwned} />
      <SoleOwnedWorkspaces
        workspaces={soleOwned}
        onDeleted={(id) =>
          setSoleOwned((current) => current.filter((workspace) => workspace.id !== id))
        }
      />
    </CenteredLayout>
  )
}
