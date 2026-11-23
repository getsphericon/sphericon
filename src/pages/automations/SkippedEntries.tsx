import { Button, Stack, TextInput } from '@mantine/core'
import { useDebouncedCallback } from '@mantine/hooks'
import { useNavigate } from '@tanstack/react-router'
import type { MouseEvent } from 'react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { AnchorLink, ButtonLink } from '../../components/RouterLink.tsx'
import { siteSkippedEntriesListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { useFormat } from '../../hooks/useFormat.ts'
import { Route as automationsDetailRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { Route as skippedEntriesRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/skipped-entries.tsx'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { withPage } from '../../searchSchemas.ts'

/** The link from an automation's report to the entries its entry checks refused. */
export function SkippedEntriesLink({
  workspaceId,
  automationId,
}: {
  workspaceId: string
  automationId: string
}) {
  const { t } = useTranslation()
  return (
    <ButtonLink
      size="xs"
      variant="default"
      to={skippedEntriesRoute.to}
      params={{ workspaceId, automationId }}
    >
      {t(($) => $.automations.skippedEntriesTitle)}
    </ButtonLink>
  )
}

/**
 * The entries the automation's entry checks refused, newest first, each with its reason, optionally
 * of one contact, whose id the route's search keeps.
 */
export function SkippedEntriesPage() {
  const { t } = useTranslation()
  const { formatDateTime } = useFormat()
  const { workspaceId, automationId } = skippedEntriesRoute.useParams()
  const { contactId, page } = skippedEntriesRoute.useSearch()
  const pageNavigate = skippedEntriesRoute.useNavigate()
  const navigate = useNavigate({ from: skippedEntriesRoute.fullPath })
  const [search, setSearch] = useState(contactId ?? '')
  // Back/forward or a link changes the URL search: the field follows it, unless it already shows
  // that contact (typing trims before it navigates).
  const [seenContact, setSeenContact] = useState(contactId)
  if (contactId !== seenContact) {
    setSeenContact(contactId)
    if (search.trim() !== (contactId ?? '')) {
      setSearch(contactId ?? '')
    }
  }
  // A filter starts again from the first page.
  const setContact = (next: string | undefined) =>
    void navigate({
      search: (prev) => ({ ...prev, contactId: next, page: undefined }),
      replace: true,
    })
  const applySearch = useDebouncedCallback((value: string) => {
    setContact(value.trim() || undefined)
  }, 300)
  const clearFilter = () => {
    setSearch('')
    setContact(undefined)
  }

  return (
    <Stack flex={1}>
      <PagedTable
        page={page}
        onPageChange={(next) =>
          void pageNavigate({ search: withPage(next), viewTransition: false })
        }
        // A new filter starts again from the first page.
        key={contactId ?? ''}
        options={(query) =>
          siteSkippedEntriesListOptions({
            path: { workspaceId, automationId },
            query: { ...query, ...(contactId ? { contactId } : {}) },
          })
        }
        loadErrorTitle={t(($) => $.automations.skippedEntriesLoadError)}
        collection={t(($) => $.automations.skippedEntriesTitle)}
        pageHeader={{
          actions: (
            <ButtonLink
              variant="subtle"
              to={automationsDetailRoute.to}
              params={{ workspaceId, automationId }}
            >
              {t(($) => $.actions.back)}
            </ButtonLink>
          ),
          count: (total) => t(($) => $.automations.skippedEntriesCount, { count: total }),
          toolbar: (
            <TextInput
              w={240}
              value={search}
              onChange={(event) => {
                setSearch(event.currentTarget.value)
                applySearch(event.currentTarget.value)
              }}
              placeholder={t(($) => $.automations.skippedEntriesContactFilter)}
              aria-label={t(($) => $.automations.skippedEntriesContactFilter)}
            />
          ),
        }}
        // A contact that matches nothing is not an automation that skipped nobody: it offers to
        // clear the filter.
        empty={
          contactId
            ? {
                title: t(($) => $.automations.noSkippedEntriesMatch),
                action: (
                  <Button variant="default" onClick={clearFilter}>
                    {t(($) => $.table.clearFilter)}
                  </Button>
                ),
              }
            : { title: t(($) => $.automations.noSkippedEntries) }
        }
        onRowClick={(record) =>
          void navigate({
            to: contactsDetailRoute.to,
            params: { workspaceId, contactId: record.contactId },
          })
        }
        columns={[
          {
            accessor: 'contactId',
            title: t(($) => $.automations.skippedEntryContact),
            render: (record) => (
              <AnchorLink
                inherit
                c="inherit"
                fw={500}
                underline="never"
                to={contactsDetailRoute.to}
                params={{ workspaceId, contactId: record.contactId }}
                onClick={(event: MouseEvent) => event.stopPropagation()}
              >
                {record.contactId}
              </AnchorLink>
            ),
          },
          {
            accessor: 'reason',
            title: t(($) => $.automations.skippedEntryReason),
            render: (record) => t(($) => $.automations.skippedReason[record.reason]),
          },
          {
            accessor: 'occurredAt',
            title: t(($) => $.automations.skippedEntryOccurredAt),
            render: (record) => formatDateTime(record.occurredAt),
          },
        ]}
      />
    </Stack>
  )
}
