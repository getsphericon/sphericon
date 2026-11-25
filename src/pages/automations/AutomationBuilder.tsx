import { Alert, Box, Button, Group, Modal, Paper, Stack, TextInput } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DraftActions } from '../../components/DraftControls.tsx'
import { PostalAddressNotice } from '../../components/PostalAddressNotice.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { ChangedNotice } from '../../editing/ChangedNotice.tsx'
import { useEditorEffects } from '../../editing/useEditorEffects.ts'
import { useEditorSession } from '../../editing/useEditorSession.ts'
import { siteAutomationsGetQueryKey } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteAutomationResource } from '../../generated/site/types.gen.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { holdsKept } from './definition.ts'
import type { Flow } from './definition.ts'
import { automationOutcomeTexts } from './draftActions.ts'
import { automationAdapter, editedFlow, editorSession } from './editorSession.ts'
import { FailedEnrollments } from './FailedEnrollments.tsx'
import { FlowCanvas } from './FlowCanvas.tsx'
import { copyStep, insertStep, removeStep, updateStep, updateTrigger } from './flowEdits.ts'
import { LeaveGuard } from './LeaveGuard.tsx'
import { StepPanel } from './StepPanel.tsx'

interface AutomationBarValues {
  name: string
}

function useAutomationBarForm(name: string) {
  const { t } = useTranslation()
  return useForm<AutomationBarValues>({
    initialValues: { name },
    validate: { name: (value) => (value.trim() === '' ? t(($) => $.validation.required) : null) },
  })
}

interface AutomationBuilderProps {
  workspaceId: string
  // The Automation whose draft, else live version, the canvas edits and a save writes over.
  automation: SiteAutomationResource
}

// useFlow holds the flow the canvas draws and the panel edits. The editor session reads it at once
// after an edit (a Save captures it before awaiting), so the latest flow is kept beside the state
// that renders it.
function useFlow(initial: Flow) {
  const latest = useRef(initial)
  const [flow, setFlow] = useState(initial)
  const write = useCallback((next: Flow) => {
    latest.current = next
    setFlow(next)
  }, [])
  const edit = useCallback(
    (change: (current: Flow) => Flow) => write(change(latest.current)),
    [write],
  )
  return { flow, latest, write, edit }
}

// AutomationBuilder hosts the flow editor. It edits the draft, or the live version when there is
// none (ADR 0065); a save writes the draft and leaves the live version alone. The name is edited in
// the bar over the canvas; the canvas draws the version's Step tree (FlowCanvas), and the side
// panel edits what is selected on it: the Trigger, with its exit and goal events, or a Step. Save
// persists the name and the flow together through the session's generated client call. Save and
// Publish preserve the canvas; only an explicit Revert replaces it. Query refreshes do not own the
// editor's local content.
export function AutomationBuilder({ workspaceId, automation }: AutomationBuilderProps) {
  const { t } = useTranslation()
  const notify = useNotify()
  const form = useAutomationBarForm(automation.name)
  const [initial] = useState(() => editedFlow(automation))
  const { flow, latest, write, edit } = useFlow(initial)
  const [selected, setSelected] = useState<string | null>(null)
  // The check of the open Step's form, which the Save runs first.
  const validatorRef = useRef<(() => boolean) | null>(null)

  const outcomeTexts = automationOutcomeTexts(t)
  const effects = useEditorEffects<SiteAutomationResource, AutomationOperation>(
    siteAutomationsGetQueryKey({ path: { workspaceId, id: automation.id } }),
    (operation) => outcomeTexts[operation],
  )
  const {
    snapshot: session,
    dirty,
    wouldLose,
    can,
    run,
    save,
    reload,
    overwrite,
  } = useEditorSession(automationAdapter, editorSession, {
    subject: { path: { workspaceId, id: automation.id }, resource: automation },
    // The bar's name and the flow hold the current content.
    editor: {
      read: () => ({ name: form.getValues().name, flow: latest.current }),
      write: (content) => {
        if (form.getValues().name !== content.name) {
          form.setValues({ name: content.name })
        }
        write(content.flow)
      },
    },
    // A refresh of the cached Automation never reloads the canvas: it updates metadata only.
    effects,
  })

  const onSave = async () => {
    if (validatorRef.current?.() === false) {
      return
    }
    await save((submission) => {
      if (form.validate().hasErrors || !submission) {
        return false
      }
      if (submission.filterIncomplete) {
        notify({
          color: 'red',
          title: t(($) => $.automations.triggerFilterIncompleteTitle),
          message: t(($) => $.automations.triggerFilterIncomplete),
        })
        return false
      }
      return true
    })
  }

  // The Automation as stored now, read again for the changed notice's two actions (ADR 0119).
  const queryClient = useQueryClient()
  const stored = async (then: (resource: SiteAutomationResource) => unknown) => {
    const queryKey = siteAutomationsGetQueryKey({ path: { workspaceId, id: automation.id } })
    await queryClient.refetchQueries({ queryKey, exact: true }, { throwOnError: true })
    const fresh = queryClient.getQueryData<SiteAutomationResource>(queryKey)
    if (fresh !== undefined) {
      await then(fresh)
    }
  }

  const content = { name: form.values.name, flow }
  const unsaved = dirty(content)
  const busy = !session.matches('idle')
  // A version holding a Step this editor does not edit (an End, a fork) is shown read-only: no edit
  // and no save could drop it. Publish and Revert act on the stored version, which keeps it.
  const kept = holdsKept(flow)
  const editable = !session.hasTag('readOnly') && !kept
  const { resource, operation } = session.context
  const sendsEmail = flow.path.steps.some((step) => step.kind === 'email')

  // The editor is a full-screen layer over the app rather than a block inside a page: the canvas
  // takes the viewport. Its focus trap is off so the menus it opens, rendered outside, stay usable.
  return (
    <Modal
      opened
      onClose={() => undefined}
      fullScreen
      padding={0}
      withCloseButton={false}
      closeOnEscape={false}
      trapFocus={false}
      transitionProps={{ duration: 0 }}
    >
      <LeaveGuard wouldLose={wouldLose(content)} />
      <Box pos="relative" w="100%" h="100dvh">
        <FlowCanvas
          flow={flow}
          editable={editable}
          selected={selected}
          select={setSelected}
          insert={(at, kind) => {
            const inserted = insertStep(latest.current, at, kind)
            write(inserted.flow)
            setSelected(inserted.id)
          }}
          remove={(id) => edit((current) => removeStep(current, id))}
        />
        <Paper withBorder shadow="sm" p="xs" pos="absolute" top={16} left={16} right={16}>
          <Group justify="space-between" wrap="nowrap">
            <Group wrap="nowrap" flex={1} miw={0}>
              <ButtonLink variant="default" to={automationsRoute.to} params={{ workspaceId }}>
                {t(($) => $.actions.back)}
              </ButtonLink>
              <TextInput
                aria-label={t(($) => $.automations.nameLabel)}
                placeholder={t(($) => $.automations.nameLabel)}
                flex={1}
                miw={0}
                maw={320}
                disabled={!editable}
                {...form.getInputProps('name')}
              />
            </Group>
            <Group wrap="nowrap" flex="none">
              <FailedEnrollments workspaceId={workspaceId} automationId={resource.id} />
              <Button
                loading={session.matches('saving')}
                disabled={busy || kept}
                onClick={() => {
                  void onSave()
                }}
              >
                {t(($) => $.actions.save)}
              </Button>
              <DraftActions
                pending={resource.live !== null && resource.draft !== null}
                publishable={can('PUBLISH')}
                publishing={operation === 'PUBLISH'}
                reverting={operation === 'REVERT'}
                disabled={busy}
                {...(unsaved && { unsaved: t(($) => $.automations.leaveMessage) })}
                onPublish={() => {
                  void run('PUBLISH')
                }}
                onRevert={() => {
                  void run('REVERT')
                }}
              />
            </Group>
          </Group>
        </Paper>
        {selected !== null && (
          <StepPanel
            // A Revert or a reload replaces the content: the panel's form opens it anew.
            key={`${selected}:${session.context.content.revision}`}
            flow={flow}
            selected={selected}
            editable={editable}
            validatorRef={validatorRef}
            onTrigger={(patch) =>
              edit((current) => updateTrigger(current, { ...current.trigger, ...patch }))
            }
            onStep={(step) => edit((current) => updateStep(current, step))}
            onCopy={(id) => {
              const copied = copyStep(latest.current, id)
              write(copied.flow)
              setSelected(copied.id)
            }}
            onClose={() => setSelected(null)}
          />
        )}
        <Stack pos="absolute" bottom={16} left={16} maw={420} gap="xs">
          {kept && (
            <Alert color="yellow" title={t(($) => $.automations.keptReadOnlyTitle)}>
              {t(($) => $.automations.keptReadOnly)}
            </Alert>
          )}
          {/* Publish is refused until the Workspace has a postal address (ADR 0156) while the
              flow holds an email step, saved or not. */}
          {sendsEmail && <PostalAddressNotice workspaceId={workspaceId} />}
          {session.context.changed && (
            <ChangedNotice
              isDirty={() => dirty({ name: form.getValues().name, flow: latest.current })}
              busy={busy}
              onLoad={() => {
                void stored(reload)
              }}
              onOverwrite={() => {
                void stored(overwrite)
              }}
            />
          )}
        </Stack>
      </Box>
    </Modal>
  )
}

type AutomationOperation = keyof typeof automationAdapter.operations
