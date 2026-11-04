import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import {
  zSiteAutomationState,
  zSiteAutomationsSort,
  zSortDirection,
} from '../../../../generated/site/zod.gen.ts'
import { archivableSearch } from '../../../../searchSchemas.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/automations/')({
  // The archived view, the name search, the state filter and the order of the list, so each is
  // linkable. Absent is the live list of every state, the newest first.
  validateSearch: archivableSearch.extend({
    name: z.coerce.string().optional().catch(undefined),
    state: zSiteAutomationState.optional().catch(undefined),
    sort: zSiteAutomationsSort.optional().catch(undefined),
    direction: zSortDirection.optional().catch(undefined),
  }),
})
