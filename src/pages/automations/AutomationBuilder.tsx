import { Box, Button, Group, Modal, Paper, TextInput, useComputedColorScheme } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useQueryClient } from '@tanstack/react-query'
import {
  closeSnackbar,
  registerComponentDecorator,
  useStore,
  useWorkflowBuilderActions,
  WorkflowBuilder,
} from '@workflowbuilder/sdk'
import type {
  DidSaveStatus,
  OnSaveExternal,
  WorkflowBuilderPlugin,
  WorkflowBuilderIsValidConnection,
} from '@workflowbuilder/sdk'
import { useCallback, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { SnapshotFrom } from 'xstate'

import { DraftActions } from '../../components/DraftControls.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { useEditorSession } from '../../editing/useEditorSession.ts'
import {
  siteAutomationsGetQueryKey,
  siteAutomationsListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteAutomationResource,
  SiteAutomationVersion,
} from '../../generated/site/types.gen.ts'
import { useApiErrorMessage } from '../../hooks/useApiErrorMessage.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { stepsToGraph } from './definition.ts'
import { automationAdapter, editorSession } from './editorSession.ts'
import type { AutomationContent, AutomationOperation } from './editorSession.ts'
import { FailedEnrollments } from './FailedEnrollments.tsx'
import { httpBodyRenderer, httpHeadersRenderer } from './httpRenderer.ts'
import {
  emailIntegrationRenderer,
  smsIntegrationRenderer,
  telegramIntegrationRenderer,
} from './integrationRenderer.ts'
import { LeaveGuard } from './LeaveGuard.tsx'
import { messageRenderer } from './messageRenderer.ts'
import { useAutomationNodeTypes } from './nodes.ts'
import { smsTextRenderer } from './smsRenderer.ts'
import { telegramTextRenderer } from './telegramRenderer.ts'
import { exitEventsRenderer, goalEventRenderer, triggerRenderer } from './triggerRenderer.ts'

// Imported as a raw string (not auto-injected): the SDK sheet carries unlayered
// global resets and .mantine-* overrides that would leak app-wide. It is rendered
// only while the builder is on screen, so it never touches the rest of the app
// (e.g. the dashboard's body scroll).
import builderStyles from '@workflowbuilder/sdk/style.css?inline'

// The email step's message is edited by the editor, not a text field, a Telegram step's text has
// its merge-tag picker and counter, an SMS step's its merge-tag picker, each send step's Integration is picked from its Channel's, the Trigger's event, exit events and goal event are typed with
// suggestions, and an HTTP step's headers are rows and its body has a dry run. A stable reference.
const jsonForm = {
  renderers: [
    messageRenderer,
    emailIntegrationRenderer,
    telegramIntegrationRenderer,
    telegramTextRenderer,
    smsIntegrationRenderer,
    smsTextRenderer,
    triggerRenderer,
    exitEventsRenderer,
    goalEventRenderer,
    httpHeadersRenderer,
    httpBodyRenderer,
  ],
}

// The SDK ships a Templates button in the palette footer, for diagrams the model cannot hold.
// A wrapper decorator that renders nothing leaves the footer empty. A stable reference.
const withoutTemplates: WorkflowBuilderPlugin = () => {
  registerComponentDecorator('OptionalFooterContent', {
    place: 'wrapper',
    content: () => null,
    name: 'automationWithoutTemplates',
  })
}
const plugins = [withoutTemplates]

// The SDK announces that it restored the saved diagram on every mount, though the diagram was
// handed to it as props. Its snackbars are keyed `<variant>:<title>:<subtitle>`.
const RESTORED_SNACKBAR_KEY = 'success:restoreDiagramSuccess:'

function QuietRestore() {
  useEffect(() => {
    const timers = [0, 100, 400].map((delay) =>
      setTimeout(() => closeSnackbar(RESTORED_SNACKBAR_KEY), delay),
    )
    return () => {
      for (const timer of timers) {
        clearTimeout(timer)
      }
    }
  }, [])

  return null
}

// The linear model has one way in: nothing connects into the Trigger.
const isValidConnection: WorkflowBuilderIsValidConnection = ({ targetNode }) =>
  !targetNode.data.isStartNode

interface AutomationBarValues {
  name: string
}
type AutomationBarForm = ReturnType<typeof useAutomationBarForm>

function useAutomationBarForm(name: string) {
  const { t } = useTranslation()
  return useForm<AutomationBarValues>({
    initialValues: { name },
    validate: { name: (value) => (value.trim() === '' ? t(($) => $.validation.required) : null) },
  })
}

interface AutomationBuilderProps {
  workspaceId: string
  automation: SiteAutomationResource
  // The version the canvas edits and a save writes over: the draft, else the live one.
  edited: SiteAutomationVersion
}

// The SDK owns its own editor theme and only auto-detects it from
// prefers-color-scheme on first mount (reactFlowProps.colorMode is blocked to
// avoid clashing with it). That misses Mantine's manual ThemeToggle, whose
// choice is persisted independently. ThemeSync bridges the two: it reads the
// computed Mantine scheme (resolving "auto" + the persisted override) and
// drives the SDK's imperative setTheme, so the canvas follows the app. Must be
// a descendant of <WorkflowBuilder.Root> for the actions hook to resolve.
function ThemeSync() {
  const { setTheme } = useWorkflowBuilderActions()
  const scheme = useComputedColorScheme('light', { getInitialValueInEffect: true })

  useEffect(() => {
    setTheme(scheme)
  }, [scheme, setTheme])

  return null
}

function EditorReadOnly({ readOnly }: { readOnly: boolean }) {
  const { setReadOnly } = useWorkflowBuilderActions()
  useEffect(() => {
    setReadOnly(readOnly)
  }, [readOnly, setReadOnly])
  return null
}

// The bar floating over the canvas replaces the SDK's own top bar: the way back, the
// automation's name, the failed Enrollments, the Save that persists the canvas to the draft, and
// Publish and Revert (ADR 0065), serialized by the editor session. It sits inside
// <WorkflowBuilder.Root> to reach the canvas.
function BuilderBar({
  form,
  workspaceId,
  session,
  dirty,
  can,
  run,
  onSave,
}: {
  form: AutomationBarForm
  workspaceId: string
  session: SnapshotFrom<typeof editorSession>
  dirty: (content: AutomationContent) => boolean
  can: (operation: AutomationOperation) => boolean
  run: (operation: AutomationOperation) => void
  onSave: () => Promise<DidSaveStatus>
}) {
  const { t } = useTranslation()
  // Subscribed, so the bar re-renders on every edit of the canvas.
  const nodes = useStore((state) => state.nodes)
  const edges = useStore((state) => state.edges)
  const { resource: automation, operation } = session.context
  const unsaved = dirty({ name: form.values.name, graph: { nodes, edges } })
  const busy = !session.matches('idle')

  return (
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
            disabled={session.hasTag('readOnly')}
            {...form.getInputProps('name')}
          />
        </Group>
        <Group wrap="nowrap" flex="none">
          <FailedEnrollments workspaceId={workspaceId} automationId={automation.id} />
          <Button
            loading={session.matches('saving')}
            disabled={busy}
            onClick={() => {
              void onSave()
            }}
          >
            {t(($) => $.actions.save)}
          </Button>
          <DraftActions
            pending={automation.live !== null && automation.draft !== null}
            publishable={can('PUBLISH')}
            publishing={operation === 'PUBLISH'}
            reverting={operation === 'REVERT'}
            disabled={busy}
            {...(unsaved && { unsaved: t(($) => $.automations.leaveMessage) })}
            onPublish={() => run('PUBLISH')}
            onRevert={() => run('REVERT')}
          />
        </Group>
      </Group>
    </Paper>
  )
}

// AutomationBuilder hosts the visual flow editor. It edits the draft, or the live version when
// there is none (ADR 0065); a save writes the draft and leaves the live version alone. The name is
// edited in the bar over the canvas; the Trigger is the canvas's first node, which also holds the
// exit and goal events. Save captures the graph as the trigger, exit and
// goal events and the linear []step definition and persists them with the name together through
// the session's generated client call. Save and Publish preserve this canvas; only explicit
// Revert replaces it. Query refreshes do not own the editor's local content.
export function AutomationBuilder({ workspaceId, automation, edited }: AutomationBuilderProps) {
  const { t } = useTranslation()
  const notify = useNotify()
  const apiErrorMessage = useApiErrorMessage()
  const queryClient = useQueryClient()
  const nodeTypes = useAutomationNodeTypes()

  const form = useAutomationBarForm(automation.name)

  const labels = useMemo(
    () =>
      Object.fromEntries(
        nodeTypes.flatMap((item) => ('type' in item ? [[item.type, item.label]] : [])),
      ),
    [nodeTypes],
  )
  const initialGraph = useMemo(() => stepsToGraph(edited, edited.steps, labels), [edited, labels])
  const listKey = siteAutomationsListQueryKey({ path: { workspaceId } })
  const getKey = siteAutomationsGetQueryKey({ path: { workspaceId, id: automation.id } })
  const {
    snapshot: session,
    dirty,
    can,
    run,
    save,
  } = useEditorSession(automationAdapter, editorSession, {
    subject: {
      path: { workspaceId, id: automation.id },
      resource: automation,
      graph: initialGraph,
      labels,
    },
    // The bar's name and the SDK's canvas store hold the current content.
    editor: {
      read: () => {
        const { nodes, edges } = useStore.getState()
        return { name: form.getValues().name, graph: { nodes, edges } }
      },
      write: ({ name, graph }) => {
        if (form.getValues().name !== name) {
          form.setValues({ name })
        }
        useStore.setState({ nodes: graph.nodes, edges: graph.edges })
      },
    },
    effects: {
      // The write's response is authoritative. An older GET is cancelled before its cache is
      // replaced; the list is reconciled in the background and never reloads the canvas.
      store: (resource) => {
        void queryClient.cancelQueries({ queryKey: getKey })
        queryClient.setQueryData(getKey, resource)
      },
      invalidate: () => {
        void queryClient.invalidateQueries({ queryKey: listKey })
      },
      accepted: (operation) => {
        const messages = {
          SAVE: t(($) => $.notifications.automationUpdated),
          PUBLISH: t(($) => $.notifications.automationPublished),
          REVERT: t(($) => $.notifications.automationReverted),
        }
        notify({
          color: 'teal',
          title: t(($) => $.notifications.successTitle),
          message: messages[operation],
        })
      },
      failed: (operation, error) => {
        notify({
          color: 'red',
          title:
            operation === 'SAVE'
              ? t(($) => $.alerts.automationSaveErrorTitle)
              : t(($) => $.alerts.automationStatusErrorTitle),
          message: apiErrorMessage(
            error,
            operation === 'SAVE'
              ? t(($) => $.alerts.automationSaveErrorMessage)
              : t(($) => $.notifications.errorMessage),
          ),
        })
      },
    },
  })

  // The bar's Save and the SDK's imperative save action share one capture and its exclusivity: the
  // session reads the canvas store, which holds what the SDK hands its save action.
  const saveCanvas = useCallback(async (): Promise<DidSaveStatus> => {
    const result = await save((submission) => {
      if (form.validate().hasErrors) {
        return false
      }
      if (!submission) {
        notify({
          color: 'red',
          title: t(($) => $.automations.triggerMissingTitle),
          message: t(($) => $.automations.triggerMissing),
        })
        return false
      }
      if (submission.dropped > 0) {
        notify({
          color: 'yellow',
          title: t(($) => $.automations.branchesUnsupportedTitle),
          message: t(($) => $.automations.branchesUnsupported),
        })
      }
      return true
    })
    return result === 'busy' ? 'alreadyStarted' : result
  }, [save, form, notify, t])

  const onDataSave = useCallback<OnSaveExternal>(async () => saveCanvas(), [saveCanvas])

  const integration = useMemo(() => ({ strategy: 'props' as const, onDataSave }), [onDataSave])

  // The SDK's canvas fills the viewport (100vw by 100vh), so the editor is a full-screen layer
  // over the app rather than a block inside a page. Its focus trap is off so the SDK's own
  // menus, rendered outside the dialog, stay usable.
  return (
    <>
      {/* The SDK stylesheet is rendered with the builder, so it is there at the first paint and
          goes when the builder does. No href or precedence: React would hoist such a sheet into
          the head and keep it after the builder is gone. */}
      <style>{builderStyles}</style>
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
        {/* Children replace the SDK's default layout: its top bar (logo, folder name,
            language, theme and read-only toggles) is swapped for <BuilderBar />, and
            ThemeSync keeps the canvas theme in step with Mantine's color scheme. */}
        <WorkflowBuilder.Root
          key={session.context.content.revision}
          name={session.context.resource.name}
          layoutDirection="DOWN"
          nodeTypes={nodeTypes}
          initialNodes={session.context.content.value.graph.nodes}
          initialEdges={session.context.content.value.graph.edges}
          integration={integration}
          isValidConnection={isValidConnection}
          jsonForm={jsonForm}
          plugins={plugins}
        >
          <ThemeSync />
          <EditorReadOnly readOnly={session.hasTag('readOnly')} />
          <QuietRestore />
          <LeaveGuard name={form.values.name} dirty={dirty} />
          <Box pos="relative" w="100%" h="100dvh">
            <WorkflowBuilder.Canvas />
            <BuilderBar
              form={form}
              workspaceId={workspaceId}
              session={session}
              dirty={dirty}
              can={can}
              run={run}
              onSave={saveCanvas}
            />
            <Box pos="absolute" top={88} left={16}>
              <WorkflowBuilder.Palette />
            </Box>
            <Box
              pos="absolute"
              top={{ base: 'auto', sm: 88 }}
              bottom={{ base: 16, sm: 'auto' }}
              right={16}
              maw="calc(100% - 32px)"
            >
              <WorkflowBuilder.PropertiesPanel />
            </Box>
          </Box>
        </WorkflowBuilder.Root>
      </Modal>
    </>
  )
}
