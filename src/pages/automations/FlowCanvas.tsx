import {
  ActionIcon,
  Box,
  Group,
  Menu,
  Paper,
  Text,
  ThemeIcon,
  UnstyledButton,
  useComputedColorScheme,
} from '@mantine/core'
import { IconAlertCircle, IconPlus, IconTrash } from '@tabler/icons-react'
import type { Icon } from '@tabler/icons-react'
import { Background, Handle, Position, ReactFlow } from '@xyflow/react'
import type { NodeProps } from '@xyflow/react'
import { createContext, use, useMemo } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { useFormat } from '../../hooks/useFormat.ts'
import type { CanvasFigures } from './canvasFigures.ts'
import { sendsMessage } from './definition.ts'
import type { Flow, FlowStep, InsertedKind } from './definition.ts'
import type { Place } from './flowEdits.ts'
import { layoutFlow, TRIGGER_NODE_ID } from './flowLayout.ts'
import type { InsertNode, PathNode, PathRole, StepNode } from './flowLayout.ts'
import { canAddPath } from './forkRules.ts'
import { insertableKinds, TRIGGER_ICON, useStepInfo, useStepKinds } from './stepKinds.ts'

// What the nodes of the canvas do: they select what the side panel edits, insert a Step at their
// place, remove theirs and add a path to a fork. Nothing on the canvas edits the flow but through
// these. `errorOf` is the error a fork's node shows once it was checked (on blur or on Save).
// `figures` are the report's, null until it is read: the nodes show none then.
interface CanvasControls {
  flow: Flow
  editable: boolean
  selected: string | null
  select: (id: string) => void
  insert: (place: Place, kind: InsertedKind) => void
  remove: (id: string) => void
  addPath: (forkId: string) => void
  errorOf: (id: string) => string | null
  figures: CanvasFigures | null
}

const Controls = createContext<CanvasControls | null>(null)

function useControls(): CanvasControls {
  const controls = use(Controls)
  if (controls === null) {
    throw new Error('A canvas node is rendered outside the canvas')
  }
  return controls
}

// A node's edges attach at its top and bottom; none is drawn or connected by hand.
function Handles() {
  return (
    <>
      <Handle type="target" position={Position.Top} isConnectable={false} />
      <Handle type="source" position={Position.Bottom} isConnectable={false} />
    </>
  )
}

// The box of the Trigger and of a Step: its icon, its kind, what it says beside it and its figures
// from the report. A click opens it in the side panel.
function NodeBox({
  id,
  icon: NodeIcon,
  label,
  detail,
  error = null,
  figures = null,
  children,
}: {
  id: string
  icon: Icon
  label: string
  detail: string
  // What is wrong with it, shown in place of the detail.
  error?: string | null
  figures?: string | null
  children?: ReactNode
}) {
  const { selected, select } = useControls()
  return (
    <Paper
      withBorder
      shadow={selected === id ? 'md' : 'xs'}
      w="100%"
      h="100%"
      px="sm"
      bd={selected === id ? '2px solid var(--mantine-primary-color-filled)' : undefined}
    >
      <Group h="100%" wrap="nowrap" gap="xs">
        <UnstyledButton flex={1} miw={0} h="100%" onClick={() => select(id)}>
          <Group wrap="nowrap" gap="sm">
            <ThemeIcon variant="light" size="lg">
              <NodeIcon size={20} />
            </ThemeIcon>
            <Box miw={0}>
              <Text size="sm" fw={500} truncate>
                {label}
              </Text>
              {error === null ? (
                detail !== '' && (
                  <Text size="xs" c="dimmed" truncate>
                    {detail}
                  </Text>
                )
              ) : (
                <Group gap={4} wrap="nowrap">
                  <IconAlertCircle size={14} color="var(--mantine-color-error)" />
                  <Text size="xs" c="red" truncate>
                    {error}
                  </Text>
                </Group>
              )}
              {figures !== null && (
                <Text size="xs" c="dimmed" truncate>
                  {figures}
                </Text>
              )}
            </Box>
          </Group>
        </UnstyledButton>
        {children}
      </Group>
    </Paper>
  )
}

function TriggerNodeView() {
  const { t } = useTranslation()
  const { flow } = useControls()
  return (
    <>
      <NodeBox
        id={TRIGGER_NODE_ID}
        icon={TRIGGER_ICON}
        label={t(($) => $.automations.stepTrigger)}
        detail={flow.trigger.action}
      />
      <Handle type="source" position={Position.Bottom} isConnectable={false} />
    </>
  )
}

// What a Step's node says beside its kind: an email's subject, a tag step's Tag, a Split's
// weights.
function stepDetail(step: FlowStep): string {
  switch (step.kind) {
    case 'email':
      return step.draft.subject
    case 'apply_tag':
    case 'remove_tag':
      return step.draft.tag
    case 'telegram':
    case 'sms':
      return step.draft.text
    case 'split':
      return step.paths.map((path) => `${path.weight === '' ? '?' : path.weight}%`).join(' / ')
    case 'http':
    case 'wait':
    case 'branch':
    case 'end':
    case 'kept':
    default:
      return ''
  }
}

// What a send Step sent and skipped, by its id over every version that had it; nothing for a Step
// that sends no message, or before the report is read.
function useStepFigures(step: FlowStep): string | null {
  const { t } = useTranslation()
  const { formatNumber } = useFormat()
  const { figures } = useControls()
  if (figures === null || !sendsMessage(step)) {
    return null
  }
  const { sent, skipped } = figures.step(step.id)
  return t(($) => $.automations.nodeFigures, {
    sent: formatNumber(sent),
    skipped: formatNumber(skipped),
  })
}

function StepNodeView({ data: { step } }: NodeProps<StepNode>) {
  const { t } = useTranslation()
  const { editable, remove, addPath, errorOf } = useControls()
  const info = useStepInfo()(step)
  const figures = useStepFigures(step)
  const fork = step.kind === 'branch' || step.kind === 'split' ? step : null
  return (
    <>
      <Handles />
      <NodeBox
        id={step.id}
        icon={info.icon}
        label={info.label}
        detail={stepDetail(step)}
        error={errorOf(step.id)}
        figures={figures}
      >
        {editable && fork !== null && (
          <ActionIcon
            variant="subtle"
            color="gray"
            aria-label={t(($) => $.automations.addPath)}
            disabled={!canAddPath(fork)}
            onClick={(event) => {
              event.stopPropagation()
              addPath(fork.id)
            }}
          >
            <IconPlus size={16} />
          </ActionIcon>
        )}
        {/* Removing a single Step needs no confirmation (docs/standards/ui.md): it is quickly
            made again. A fork whose paths hold Steps asks first (remove). */}
        {editable && step.kind !== 'kept' && (
          <ActionIcon
            variant="subtle"
            color="gray"
            aria-label={t(($) => $.automations.removeStep)}
            onClick={(event) => {
              // The node's own click opens it; removing it must not.
              event.stopPropagation()
              remove(step.id)
            }}
          >
            <IconTrash size={16} />
          </ActionIcon>
        )}
      </NodeBox>
    </>
  )
}

// What heads a path of a fork: its place among the Branch's Conditions, `otherwise`, the unknown
// path, or a Split's path and its weight.
function usePathLabel(): (role: PathRole) => string {
  const { t } = useTranslation()
  return (role) => {
    switch (role.kind) {
      case 'condition':
        return t(($) => $.automations.pathNumbered, { number: role.index + 1 })
      case 'otherwise':
        return t(($) => $.automations.pathOtherwise)
      case 'unknown':
        return t(($) => $.automations.pathUnknown)
      case 'weight':
      default:
        return t(($) => $.automations.pathWeighted, {
          number: role.index + 1,
          weight: role.weight === '' ? '?' : role.weight,
        })
    }
  }
}

// The head of a fork's path, under the fork: what heads it and, once the report is read, the
// Enrollments that took it and those among them that reached the goal. A click opens the fork in
// the side panel.
function PathNodeView({ data: { fork, pathId, role } }: NodeProps<PathNode>) {
  const { t } = useTranslation()
  const { formatNumber } = useFormat()
  const { select, figures } = useControls()
  const label = usePathLabel()(role)
  const counts = figures?.path(fork.id, pathId) ?? null
  return (
    <>
      <Handles />
      <Paper withBorder radius="xl" w="100%" h="100%" px="sm">
        <UnstyledButton w="100%" h="100%" onClick={() => select(fork.id)}>
          <Text size="xs" fw={500} ta="center" truncate>
            {label}
          </Text>
          {counts !== null && (
            <Text size="xs" c="dimmed" ta="center" truncate>
              {t(($) => $.automations.pathFigures, {
                enrollments: formatNumber(counts.enrollments),
                goalReached: formatNumber(counts.goalReached),
              })}
            </Text>
          )}
        </UnstyledButton>
      </Paper>
    </>
  )
}

// An insertion point on an edge or in an empty path: it offers each kind of Step, and puts the one
// chosen there. An End is offered only at the end of a path.
function InsertNodeView({ data: { path, at, end } }: NodeProps<InsertNode>) {
  const { t } = useTranslation()
  const { editable, insert } = useControls()
  const kinds = insertableKinds(useStepKinds(), end)
  return (
    <>
      <Handles />
      {editable && (
        <Menu position="right-start" withinPortal>
          <Menu.Target>
            <ActionIcon
              variant="default"
              radius="xl"
              size={28}
              aria-label={t(($) => $.automations.insertStep)}
            >
              <IconPlus size={16} />
            </ActionIcon>
          </Menu.Target>
          <Menu.Dropdown>
            {kinds.map(({ kind, label, icon: KindIcon }) => (
              <Menu.Item
                key={kind}
                leftSection={<KindIcon size={16} />}
                onClick={() => insert({ path, at }, kind)}
              >
                {label}
              </Menu.Item>
            ))}
          </Menu.Dropdown>
        </Menu>
      )}
    </>
  )
}

// Stable references: the canvas re-registers its node kinds when this object changes.
const nodeTypes = {
  trigger: TriggerNodeView,
  step: StepNodeView,
  path: PathNodeView,
  insert: InsertNodeView,
}

// FlowCanvas draws the flow top to bottom, laid out anew on every change (flowLayout.ts). Its nodes
// are neither dragged nor connected: the canvas cannot hold a shape the server would refuse.
export function FlowCanvas(controls: CanvasControls) {
  const { flow } = controls
  const { nodes, edges } = useMemo(() => layoutFlow(flow), [flow])
  const scheme = useComputedColorScheme('light', { getInitialValueInEffect: true })

  return (
    <Controls value={controls}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        colorMode={scheme}
        nodesDraggable={false}
        nodesConnectable={false}
        nodesFocusable={false}
        edgesFocusable={false}
        elementsSelectable={false}
        deleteKeyCode={null}
        // A click on the Trigger or a Step opens it in the side panel, one on a path's head its
        // fork; an insertion point is no node to open.
        onNodeClick={(_, node) => {
          if (node.type === 'path') {
            controls.select(node.data.fork.id)
          } else if (node.type !== 'insert') {
            controls.select(node.id)
          }
        }}
        fitView
        fitViewOptions={{ maxZoom: 1 }}
      >
        <Background />
      </ReactFlow>
    </Controls>
  )
}
