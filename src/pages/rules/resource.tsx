import { useTranslation } from 'react-i18next'

import { DraftActions } from '../../components/DraftControls.tsx'
import {
  siteRulesCreateMutation,
  siteRulesGetOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { siteRulesUpdate } from '../../generated/site/sdk.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import { withRevision } from '../../resources/resourceEditor.ts'
import { Route as rulesEditRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/edit.tsx'
import { RuleDryRun } from './RuleDryRun.tsx'
import { editedRule, ruleOperations, ruleSchema } from './ruleEditor.ts'
import { RuleForm } from './RuleForm.tsx'

const ruleResource = defineResource({
  schema: ruleSchema,
  Form: RuleForm,
  getOptions: siteRulesGetOptions,
  createMutation: siteRulesCreateMutation,
  update: withRevision(siteRulesUpdate),
  operations: ruleOperations,
  editRoute: rulesEditRoute,
  idParam: 'ruleId',
  texts: (t) => ({
    createTitle: t(($) => $.rules.createTitle),
    editTitle: t(($) => $.rules.editTitle),
    created: t(($) => $.notifications.ruleCreated),
    updated: t(($) => $.notifications.ruleUpdated),
    loadErrorTitle: t(($) => $.alerts.ruleLoadErrorTitle),
    saveErrorTitle: t(($) => $.alerts.ruleSaveErrorTitle),
  }),
  editTarget: (created, { workspaceId }) => ({
    to: rulesEditRoute.to,
    params: { workspaceId, ruleId: created.id },
  }),
  editedValues: editedRule,
})

// Function declarations (hoisted) because the route files and this module import each other.
export function RuleCreatePage() {
  return <ruleResource.CreatePage />
}

export function RuleEditPage() {
  const { t } = useTranslation()
  const { workspaceId, ruleId } = rulesEditRoute.useParams()
  return (
    <ruleResource.EditPage>
      {(rule, editor) => (
        <>
          <DraftActions
            pending={rule.live !== null && rule.draft !== null}
            publishable={editor.can('PUBLISH')}
            publishing={editor.operation === 'PUBLISH'}
            reverting={editor.operation === 'REVERT'}
            disabled={editor.busy}
            {...(editor.dirty && { unsaved: t(($) => $.leave.message) })}
            onPublish={() => {
              void editor.run('PUBLISH')
            }}
            onRevert={() => {
              void editor.run('REVERT')
            }}
          />
          <RuleDryRun workspaceId={workspaceId} ruleId={ruleId} />
        </>
      )}
    </ruleResource.EditPage>
  )
}
