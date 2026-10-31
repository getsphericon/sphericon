import {
  Box,
  Button,
  Group,
  Modal,
  Paper,
  Select,
  TextInput,
  useComputedColorScheme,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  closeSnackbar,
  registerComponentDecorator,
  useStore,
  useWorkflowBuilderActions,
  WorkflowBuilder,
} from '@workflowbuilder/sdk'
import type {
  DidSaveStatus,
  IntegrationDataFormat,
  OnSaveExternal,
  WorkflowBuilderPlugin,
  WorkflowBuilderIsValidConnection,
} from '@workflowbuilder/sdk'
import i18next from 'i18next'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { I18nextProvider, useTranslation } from 'react-i18next'

import { DraftControls } from '../../components/DraftControls.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteAutomationsGetQueryKey,
  siteAutomationsListQueryKey,
  siteAutomationsUpdateMutation,
  siteIntegrationsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteAutomationResource,
  SiteAutomationVersion,
} from '../../generated/site/types.gen.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { i18n } from '../../i18n.ts'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { getApiErrorMessage } from '../../utils/apiErrors.ts'
import { graphToSteps, stepsToGraph } from './definition.ts'
import type { AutomationGraph } from './definition.ts'
import { automationDraftActions } from './draftActions.ts'
import { FailedEnrollments } from './FailedEnrollments.tsx'
import { LeaveGuard } from './LeaveGuard.tsx'
import { automationSnapshot } from './leaveSnapshot.ts'
import { messageRenderer } from './messageRenderer.ts'
import { useAutomationNodeTypes } from './nodes.ts'
import { exitEventsRenderer, goalEventRenderer, triggerRenderer } from './triggerRenderer.ts'

// Imported as a raw string (not auto-injected): the SDK sheet carries unlayered
// global resets and .mantine-* overrides that would leak app-wide. It is rendered
// only while the builder is on screen, so it never touches the rest of the app
// (e.g. the dashboard's body scroll).
import builderStyles from '@workflowbuilder/sdk/style.css?inline'

// The email step's message is edited by the editor, not a text field, and the Trigger's event,
// exit events and goal event are typed with suggestions. A stable reference.
const jsonForm = {
  renderers: [messageRenderer, triggerRenderer, exitEventsRenderer, goalEventRenderer],
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

// The picker's value for "no Integration named", which sends through the Workspace default.
const DEFAULT_INTEGRATION = 'default'

interface AutomationBarValues {
  name: string
  integrationId: string | null
}
type AutomationBarForm = ReturnType<typeof useAutomationBarForm>

function useAutomationBarForm(name: string, integrationId: string | null) {
  const { t } = useTranslation()
  return useForm<AutomationBarValues>({
    initialValues: { name, integrationId },
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

// The bar floating over the canvas replaces the SDK's own top bar: the way back, the
// automation's name, the failed Enrollments, the Save that persists the canvas to the draft, and
// Publish and Revert (ADR 0065), which wait for unsaved changes to be saved. It sits inside
// <WorkflowBuilder.Root> to reach the save action and the canvas, and under the SDK's i18next
// instance, which does not hold the app's strings, so it is bound back to the app's instance.
function BuilderBar({
  form,
  workspaceId,
  automation,
  saved,
  onSave,
}: {
  form: AutomationBarForm
  workspaceId: string
  automation: SiteAutomationResource
  saved: string
  onSave: (graph: AutomationGraph) => Promise<DidSaveStatus>
}) {
  const { t } = useTranslation()
  const canvasNodes = useStore((state) => state.nodes)
  const canvasEdges = useStore((state) => state.edges)
  const dirty =
    automationSnapshot(form.values.name, form.values.integrationId, {
      nodes: canvasNodes,
      edges: canvasEdges,
    }) !== saved
  const integrationsQuery = useQuery(
    siteIntegrationsListOptions({ path: { workspaceId }, query: { archived: false } }),
  )
  const integrations = [
    { value: DEFAULT_INTEGRATION, label: t(($) => $.automations.integrationDefault) },
    ...(integrationsQuery.data ?? [])
      .filter((integration) => integration.channel === 'email')
      .map((integration) => ({ value: integration.id, label: integration.name })),
  ]
  const saveMutation = useMutation({
    mutationFn: async () => {
      const { nodes, edges } = useStore.getState()
      return onSave({ nodes, edges })
    },
  })

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
            {...form.getInputProps('name')}
          />
        </Group>
        <Group wrap="nowrap" flex="none">
          <Select
            aria-label={t(($) => $.automations.integrationLabel)}
            placeholder={t(($) => $.automations.integrationLabel)}
            data={integrations}
            allowDeselect={false}
            value={form.values.integrationId ?? DEFAULT_INTEGRATION}
            onChange={(value) =>
              form.setFieldValue(
                'integrationId',
                value === null || value === DEFAULT_INTEGRATION ? null : value,
              )
            }
          />
          <FailedEnrollments workspaceId={workspaceId} automationId={automation.id} />
          <Button
            loading={saveMutation.isPending}
            onClick={() => {
              if (!form.validate().hasErrors) {
                saveMutation.mutate()
              }
            }}
          >
            {t(($) => $.actions.save)}
          </Button>
          <DraftControls
            workspaceId={workspaceId}
            resource={automation}
            dirty={dirty}
            {...automationDraftActions(t)}
          />
        </Group>
      </Group>
    </Paper>
  )
}

// AutomationBuilder hosts the visual flow editor. It edits the draft, or the live version when
// there is none (ADR 0065); a save writes the draft and leaves the live version alone. The name is
// edited in the bar over the canvas; the Trigger is the canvas's first node, which also holds the
// exit and goal events. Save drives onDataSave, which converts the graph to the trigger, exit and
// goal events and the linear []step definition and persists them with the name together through
// the generated update mutation. The page remounts it when the version it edits changes (a
// publish or a revert).
export function AutomationBuilder({ workspaceId, automation, edited }: AutomationBuilderProps) {
  const { t } = useTranslation()
  const notify = useNotify()
  const queryClient = useQueryClient()
  const nodeTypes = useAutomationNodeTypes()

  const form = useAutomationBarForm(automation.name, edited.integrationId ?? null)

  const initialGraph = useMemo(() => {
    const labels = Object.fromEntries(
      nodeTypes.flatMap((item) => ('type' in item ? [[item.type, item.label]] : [])),
    )
    return stepsToGraph(edited, edited.steps, labels)
  }, [edited, nodeTypes])

  const [saved, setSaved] = useState(() =>
    automationSnapshot(automation.name, edited.integrationId ?? null, initialGraph),
  )

  const updateMutation = useMutation({
    ...siteAutomationsUpdateMutation(),
    onSuccess: async (updated) => {
      await queryClient.invalidateQueries({
        queryKey: siteAutomationsListQueryKey({ path: { workspaceId } }),
      })
      await queryClient.invalidateQueries({
        queryKey: siteAutomationsGetQueryKey({ path: { workspaceId, id: updated.id } }),
      })
    },
  })

  const saveGraph = useCallback(
    async (graph: AutomationGraph): Promise<DidSaveStatus> => {
      const { triggerEvent, exitEvents, goalEvent, steps, dropped } = graphToSteps(graph)
      if (triggerEvent === null) {
        notify({
          color: 'red',
          title: t(($) => $.automations.triggerMissingTitle),
          message: t(($) => $.automations.triggerMissing),
        })
        return 'error'
      }
      if (dropped > 0) {
        notify({
          color: 'yellow',
          title: t(($) => $.automations.branchesUnsupportedTitle),
          message: t(($) => $.automations.branchesUnsupported),
        })
      }
      const values = form.getValues()
      try {
        await updateMutation.mutateAsync({
          path: { workspaceId, id: automation.id },
          body: {
            name: values.name.trim(),
            triggerEvent,
            exitEvents,
            goalEvent,
            steps,
            integrationId: values.integrationId,
          },
        })
        setSaved(automationSnapshot(values.name, values.integrationId, graph))
        notify({
          color: 'teal',
          title: t(($) => $.notifications.successTitle),
          message: t(($) => $.notifications.automationUpdated),
        })
        return 'success'
      } catch (error) {
        notify({
          color: 'red',
          title: t(($) => $.alerts.automationSaveErrorTitle),
          message: getApiErrorMessage(
            error,
            t(($) => $.alerts.automationSaveErrorMessage),
          ),
        })
        return 'error'
      }
    },
    [workspaceId, automation.id, form, updateMutation, notify, t],
  )

  // The SDK's own save (its shortcut) answers with its success snackbar whatever this returns, so
  // the bar saves through `saveGraph` itself and only the app's toasts show.
  const onDataSave = useCallback<OnSaveExternal>(
    async (data: IntegrationDataFormat) => saveGraph({ nodes: data.nodes, edges: data.edges }),
    [saveGraph],
  )

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
        {/* The SDK initialises and reads the global i18next instance for its own
          strings. The app tree runs on a private instance (see main.tsx), so the
          builder subtree is bound back to the global one here. */}
        <I18nextProvider i18n={i18next}>
          {/* Children replace the SDK's default layout: its top bar (logo, folder name,
            language, theme and read-only toggles) is swapped for <BuilderBar />, and
            ThemeSync keeps the canvas theme in step with Mantine's color scheme. */}
          <WorkflowBuilder.Root
            key={automation.id}
            name={automation.name}
            layoutDirection="DOWN"
            nodeTypes={nodeTypes}
            initialNodes={initialGraph.nodes}
            initialEdges={initialGraph.edges}
            integration={integration}
            isValidConnection={isValidConnection}
            jsonForm={jsonForm}
            plugins={plugins}
          >
            <ThemeSync />
            <QuietRestore />
            <LeaveGuard
              name={form.values.name}
              integrationId={form.values.integrationId}
              saved={saved}
            />
            <Box pos="relative" w="100%" h="100dvh">
              <WorkflowBuilder.Canvas />
              <I18nextProvider i18n={i18n}>
                <BuilderBar
                  form={form}
                  workspaceId={workspaceId}
                  automation={automation}
                  saved={saved}
                  onSave={saveGraph}
                />
              </I18nextProvider>
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
        </I18nextProvider>
      </Modal>
    </>
  )
}
