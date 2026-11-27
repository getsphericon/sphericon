import { Alert, Box, Button, Group, Modal, Paper, Stack, TextInput } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DraftActions } from '../../components/DraftControls.tsx'
import { PostalAddressNotice } from '../../components/PostalAddressNotice.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { ChangedNotice } from '../../editing/ChangedNotice.tsx'
import { useEditorEffects } from '../../editing/useEditorEffects.ts'
import { useEditorSession } from '../../editing/useEditorSession.ts'
import {
  siteAutomationReportsGetOptions,
  siteAutomationsGetQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteAutomationResource } from '../../generated/site/types.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useNotify } from '../../hooks/useNotify.ts'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { canvasFigures } from './canvasFigures.ts'
import { allSteps, forkPaths, holdsKept, storedNames } from './definition.ts'
import type { Flow } from './definition.ts'
import { automationOutcomeTexts } from './draftActions.ts'
import {
  automationAdapter,
  automationSubmission,
  editedFlow,
  editorSession,
} from './editorSession.ts'
import { FailedEnrollments } from './FailedEnrollments.tsx'
import { FlowCanvas } from './FlowCanvas.tsx'
import {
  addPath,
  copyStep,
  insertStep,
  movePath,
  nestedSteps,
  removePath,
  removeStep,
  setAwaitedForm,
  setUnknownPath,
  updateFork,
  updateStep,
  updateTrigger,
  updateWaitForEvent,
} from './flowEdits.ts'
import { TRIGGER_NODE_ID } from './flowLayout.ts'
import { LeaveGuard } from './LeaveGuard.tsx'
import { useNodeErrors } from './nodeErrors.ts'
import { refusedNodes } from './refusedNodes.ts'
import type { NodeRefusal } from './refusedNodes.ts'
import { useStepInfo } from './stepKinds.ts'
import { StepPanel } from './StepPanel.tsx'
import type { ForkControls } from './StepPanel.tsx'
import { useUnknownPaths } from './useUnknownPaths.ts'

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
  // The nodes checked on blur or on Save (the Trigger, forks, Wait until Steps): they show their
  // errors from then on.
  const [checked, setChecked] = useState<ReadonlySet<string>>(new Set())
  const check = useCallback(
    (ids: string[]) => setChecked((current) => new Set([...current, ...ids])),
    [],
  )
  const confirm = useConfirmation()
  const stepInfo = useStepInfo()
  // The figures each node and path shows, over the Automation's whole history. They are no part of
  // what is edited: until they are read, or when they cannot be, the canvas shows none.
  const reportQuery = useQuery(
    siteAutomationReportsGetOptions({ path: { workspaceId, automationId: automation.id } }),
  )
  const report = reportQuery.data
  const figures = useMemo(() => (report === undefined ? null : canvasFigures(report)), [report])

  // The body the last Save sent, and the nodes a refusal of it is about, each with the server's
  // message: shown on the node and in its panel until the node is edited or Save is run again.
  const sent = useRef<unknown>(null)
  const [refused, setRefused] = useState<ReadonlyMap<string, NodeRefusal>>(new Map())
  const unrefuse = (id: string) =>
    setRefused((current) => {
      if (!current.has(id)) {
        return current
      }
      const next = new Map(current)
      next.delete(id)
      return next
    })

  const outcomeTexts = automationOutcomeTexts(t)
  const editorEffects = useEditorEffects<SiteAutomationResource, AutomationOperation>(
    siteAutomationsGetQueryKey({ path: { workspaceId, id: automation.id } }),
    (operation) => outcomeTexts[operation],
  )
  // A refused Save whose errors are each about a node shows them there (docs/standards/ui.md,
  // Errors: under what failed), the first refused node opened; any other failure is a toast.
  const effects: typeof editorEffects = {
    ...editorEffects,
    failed: (operation, error) => {
      const nodes = operation === 'SAVE' ? refusedNodes(error, sent.current) : null
      if (nodes === null) {
        editorEffects.failed(operation, error)
        return
      }
      setRefused(nodes)
      setSelected(nodes.keys().next().value ?? null)
    },
  }
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

  const nodeErrors = useNodeErrors(flow)

  // The check a Save and a "Save mine over it" run before they send: every node is checked (the
  // Trigger's date, forks, Wait until Steps), its panel open or not, and it shows its error from
  // then on; the open panel's form is checked; the first invalid node whose panel is closed opens.
  // False when anything is invalid.
  const checkBeforeSending = (): boolean => {
    const invalid = nodeErrors(latest.current)
    check(invalid.map((node) => node.id))
    if (validatorRef.current?.() === false) {
      return false
    }
    const [first] = invalid
    if (first !== undefined) {
      setSelected(first.id)
      return false
    }
    return true
  }

  const onSave = async () => {
    setRefused(new Map())
    if (!checkBeforeSending()) {
      return
    }
    await save((submission) => {
      if (form.validate().hasErrors || !submission) {
        return false
      }
      sent.current = submission.body
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
  // A version holding a Step of a kind newer than this editor is shown read-only: no edit and no
  // save could drop it. Publish and Revert act on the stored version, which keeps it.
  const kept = holdsKept(flow)
  const editable = !session.hasTag('readOnly') && !kept
  const { resource, operation } = session.context
  const sendsEmail = allSteps(flow.path).some((step) => step.kind === 'email')
  // The Wait for event names stored under their Steps' ids, as the last read or acknowledged
  // Automation holds them.
  const names = useMemo(() => storedNames([resource.draft, resource.live]), [resource])

  useUnknownPaths({
    workspaceId,
    flow,
    editable,
    setUnknown: (id, present) => edit((current) => setUnknownPath(current, id, present)),
    confirmRemoval: (branch) =>
      confirm({
        title: t(($) => $.automations.removeUnknownPathTitle),
        description: t(($) => $.automations.removeUnknownPathConfirm, {
          count: branch.unknown?.steps.length ?? 0,
        }),
        confirmLabel: t(($) => $.automations.removeConfirm),
        onConfirm: () => edit((current) => setUnknownPath(current, branch.id, false)),
      }),
  })

  // Removing a single Step or an empty path needs no confirmation (docs/standards/ui.md): it is
  // quickly made again. A fork or a path holding Steps asks first, naming how many go with it.
  const removeWithSteps = (count: number, label: string, onConfirm: () => void) => {
    if (count === 0) {
      onConfirm()
      return
    }
    confirm({
      title: t(($) => $.automations.removeWithStepsTitle, { label }),
      description: t(($) => $.automations.removeWithStepsConfirm, { count, label }),
      confirmLabel: t(($) => $.automations.removeConfirm),
      onConfirm,
    })
  }
  const errors = nodeErrors(flow)
  const errorOf = (id: string): string | null =>
    (checked.has(id) ? errors.find((node) => node.id === id)?.message : undefined) ??
    refused.get(id)?.message ??
    null
  const forks: ForkControls = {
    checked: (id) => checked.has(id),
    onChecked: (id) => check([id]),
    storedName: (id) => names.get(id),
    onWaitForEvent: (id, draft) => {
      unrefuse(id)
      edit((current) => updateWaitForEvent(current, id, draft))
    },
    // The Event form is never unknown: switching to it removes the unknown path, asking first when
    // the path holds Steps, naming how many go with it.
    onAwaitedForm: (id, awaited) => {
      unrefuse(id)
      const switched = () => edit((current) => setAwaitedForm(current, id, awaited))
      const step = allSteps(latest.current.path).find((s) => s.id === id)
      const unknown = step?.kind === 'wait_for_event' ? step.unknown : null
      const count = awaited === 'event' && unknown !== null ? allSteps(unknown).length : 0
      if (count === 0) {
        switched()
        return
      }
      confirm({
        title: t(($) => $.automations.removeUnknownPathTitle),
        description: t(($) => $.automations.waitForEvent.removeUnknownPathConfirm, { count }),
        confirmLabel: t(($) => $.automations.removeConfirm),
        onConfirm: switched,
      })
    },
    onPaths: (id, patches) => edit((current) => updateFork(current, id, patches)),
    onAddPath: (id) => edit((current) => addPath(current, id).flow),
    onRemovePath: (id, pathId) => {
      const fork = allSteps(latest.current.path).find((step) => step.id === id)
      const path = fork === undefined ? undefined : forkPaths(fork).find((p) => p.id === pathId)
      removeWithSteps(
        path === undefined ? 0 : allSteps(path).length,
        t(($) => $.automations.pathLabel),
        () => edit((current) => removePath(current, id, pathId)),
      )
    },
    onMovePath: (id, move) => edit((current) => movePath(current, id, move)),
  }

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
          insert={(place, kind) => {
            const inserted = insertStep(latest.current, place, kind)
            write(inserted.flow)
            setSelected(inserted.id)
          }}
          remove={(id) => {
            const step = allSteps(latest.current.path).find((s) => s.id === id)
            removeWithSteps(
              step === undefined ? 0 : nestedSteps(step),
              step === undefined ? '' : stepInfo(step).label,
              () => edit((current) => removeStep(current, id)),
            )
          }}
          addPath={(id) => edit((current) => addPath(current, id).flow)}
          errorOf={errorOf}
          figures={figures}
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
            // The panel's form opens anew on another node, whose form is seeded from that node
            // alone (two Steps of a kind never share its values), and after a Revert or a reload,
            // which replace the content.
            key={`${selected}:${session.context.content.revision}`}
            flow={flow}
            selected={selected}
            editable={editable}
            validatorRef={validatorRef}
            refusal={refused.get(selected) ?? null}
            onTrigger={(patch) => {
              unrefuse(TRIGGER_NODE_ID)
              edit((current) => updateTrigger(current, { ...current.trigger, ...patch }))
            }}
            onStep={(step) => {
              unrefuse(step.id)
              edit((current) => updateStep(current, step))
            }}
            forks={forks}
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
                setRefused(new Map())
                if (checkBeforeSending()) {
                  // What it sends: the same content, on the newer revision.
                  sent.current =
                    automationSubmission(form.getValues().name, latest.current)?.body ?? null
                  void stored(overwrite)
                }
              }}
            />
          )}
        </Stack>
      </Box>
    </Modal>
  )
}

type AutomationOperation = keyof typeof automationAdapter.operations
