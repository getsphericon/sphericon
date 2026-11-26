import {
  siteAuditGetRetentionOptions,
  siteWorkspacesGetConfirmationEmailOptions,
  siteWorkspacesGetOptions,
  siteWorkspacesGetRevenueSettingsOptions,
  siteWorkspacesGetSendingRulesOptions,
} from '../../../generated/site/@tanstack/react-query.gen.ts'
import {
  siteAuditSetRetention,
  siteWorkspacesSetTimezone,
  siteWorkspacesUpdate,
  siteWorkspacesUpdateConfirmationEmail,
  siteWorkspacesReplaceSendingRules,
  siteWorkspacesUpdateRevenueSettings,
} from '../../../generated/site/sdk.gen.ts'
import { defineEditForm, defineModalEditor } from '../../../resources/defineResource.tsx'
import { withRevision } from '../../../resources/resourceEditor.ts'
import { formPathOf, sendingRulesFormSchema } from './sendingRules.ts'
import { SendingRulesFields } from './SendingRulesFields.tsx'
import {
  ConfirmationEmailFields,
  GeneralFields,
  RetentionFields,
  RevenueFields,
  TimezoneFields,
} from './WorkspaceSettingsFields.tsx'
import {
  confirmationEmailSchema,
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

// The confirmation email of confirmed opt-in (ADR 0013).
export const ConfirmationEmailEditForm = defineEditForm({
  editorId: 'workspaceConfirmationEmailEditor',
  schema: confirmationEmailSchema,
  Form: ConfirmationEmailFields,
  getOptions: ofWorkspace(siteWorkspacesGetConfirmationEmailOptions),
  update: withRevision(async ({ path, body, throwOnError }) =>
    siteWorkspacesUpdateConfirmationEmail({
      path: { workspaceId: path.workspaceId },
      body,
      throwOnError,
    }),
  ),
  operations: {},
  texts: (t) => ({
    editTitle: t(($) => $.settings.confirmedOptIn.emailTitle),
    updated: t(($) => $.settings.saved),
    loadErrorTitle: t(($) => $.settings.confirmedOptIn.loadError),
    saveErrorTitle: t(($) => $.settings.confirmedOptIn.saveErrorTitle),
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

// The Workspace's Frequency caps and Sending windows (ADR 0134, ADR 0137), in a modal over their
// lines: a refusal of a field shows under it, the form naming a window's day by its day.
export const sendingRulesEditor = defineModalEditor({
  editorId: 'workspaceSendingRulesEditor',
  schema: sendingRulesFormSchema,
  Form: SendingRulesFields,
  getOptions: ofWorkspace(siteWorkspacesGetSendingRulesOptions),
  update: withRevision(async ({ path, body, throwOnError }) =>
    siteWorkspacesReplaceSendingRules({
      path: { workspaceId: path.workspaceId },
      body,
      throwOnError,
    }),
  ),
  operations: {},
  refusalField: formPathOf,
  texts: (t) => ({
    editTitle: t(($) => $.settings.sendingRules.formTitle),
    updated: t(($) => $.settings.sendingRules.saved),
    loadErrorTitle: t(($) => $.settings.sendingRules.loadError),
    saveErrorTitle: t(($) => $.settings.sendingRules.saveError),
  }),
})
