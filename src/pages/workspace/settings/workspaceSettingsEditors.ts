import {
  siteAuditGetRetentionOptions,
  siteWorkspacesGetOptions,
  siteWorkspacesGetRevenueSettingsOptions,
} from '../../../generated/site/@tanstack/react-query.gen.ts'
import {
  siteAuditSetRetention,
  siteWorkspacesSetTimezone,
  siteWorkspacesUpdate,
  siteWorkspacesUpdateRevenueSettings,
} from '../../../generated/site/sdk.gen.ts'
import { defineEditForm, defineModalEditor } from '../../../resources/defineResource.tsx'
import { withRevision } from '../../../resources/resourceEditor.ts'
import {
  GeneralFields,
  RetentionFields,
  RevenueFields,
  TimezoneFields,
} from './WorkspaceSettingsFields.tsx'
import {
  generalSchema,
  retentionSchema,
  revenueSchema,
  timezoneSchema,
} from './workspaceSettingsForm.ts'

// The Workspace's settings are addressed by the Workspace alone: the path's id is unused. Each
// section is an editor session over the one revision of the Workspace's row (ADR 0119); a section
// saved leaves the others able to save, since what each shows did not change (docs/standards/ui.md).
const ofWorkspace =
  <T>(call: (options: { path: { workspaceId: string } }) => T) =>
  ({ path }: { path: { workspaceId: string; id: string } }) =>
    call({ path: { workspaceId: path.workspaceId } })

// The Workspace's name and postal address.
export const GeneralEditForm = defineEditForm({
  editorId: 'workspaceGeneralEditor',
  schema: generalSchema,
  Form: GeneralFields,
  getOptions: ofWorkspace(siteWorkspacesGetOptions),
  update: withRevision(async ({ path, body, throwOnError }) =>
    siteWorkspacesUpdate({ path: { workspaceId: path.workspaceId }, body, throwOnError }),
  ),
  operations: {},
  texts: (t) => ({
    editTitle: t(($) => $.settings.nav.general),
    updated: t(($) => $.settings.saved),
    loadErrorTitle: t(($) => $.settings.errorTitle),
    saveErrorTitle: t(($) => $.settings.errorTitle),
  }),
})

// The Workspace time zone (ADR 0057).
export const TimezoneEditForm = defineEditForm({
  editorId: 'workspaceTimezoneEditor',
  schema: timezoneSchema,
  Form: TimezoneFields,
  getOptions: ofWorkspace(siteWorkspacesGetOptions),
  update: withRevision(async ({ path, body, throwOnError }) =>
    siteWorkspacesSetTimezone({ path: { workspaceId: path.workspaceId }, body, throwOnError }),
  ),
  operations: {},
  texts: (t) => ({
    editTitle: t(($) => $.settings.timezoneLabel),
    updated: t(($) => $.settings.saved),
    loadErrorTitle: t(($) => $.settings.errorTitle),
    saveErrorTitle: t(($) => $.settings.errorTitle),
  }),
})

// The Reporting currency and the Attribution window (ADR 0064).
export const RevenueEditForm = defineEditForm({
  editorId: 'workspaceRevenueEditor',
  schema: revenueSchema,
  Form: RevenueFields,
  getOptions: ofWorkspace(siteWorkspacesGetRevenueSettingsOptions),
  update: withRevision(async ({ path, body, throwOnError }) =>
    siteWorkspacesUpdateRevenueSettings({
      path: { workspaceId: path.workspaceId },
      body,
      throwOnError,
    }),
  ),
  operations: {},
  texts: (t) => ({
    editTitle: t(($) => $.settings.revenue.title),
    updated: t(($) => $.settings.saved),
    loadErrorTitle: t(($) => $.settings.revenue.loadError),
    saveErrorTitle: t(($) => $.settings.revenue.errorTitle),
  }),
})

// The Audit log's retention window, in a modal over the log.
export const retentionEditor = defineModalEditor({
  editorId: 'auditRetentionEditor',
  schema: retentionSchema,
  Form: RetentionFields,
  getOptions: ofWorkspace(siteAuditGetRetentionOptions),
  update: withRevision(async ({ path, body, throwOnError }) =>
    siteAuditSetRetention({ path: { workspaceId: path.workspaceId }, body, throwOnError }),
  ),
  operations: {},
  texts: (t) => ({
    editTitle: t(($) => $.settings.auditLog.retentionTitle),
    updated: t(($) => $.settings.auditLog.retentionSaved),
    loadErrorTitle: t(($) => $.settings.auditLog.retentionError),
    saveErrorTitle: t(($) => $.settings.auditLog.retentionError),
  }),
})
