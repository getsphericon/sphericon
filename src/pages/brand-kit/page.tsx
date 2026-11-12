import {
  Button,
  ColorInput,
  Fieldset,
  Group,
  NumberInput,
  SimpleGrid,
  Stack,
  Text,
  Textarea,
  TextInput,
  Title,
} from '@mantine/core'
import { modals } from '@mantine/modals'
import { useQueryClient, useSuspenseQueries } from '@tanstack/react-query'
import { createContext, use } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../../components/Skeletons.tsx'
import {
  siteBrandKitGetOptions,
  siteBrandKitRecreateStarterTemplatesMutation,
  siteTemplatesListQueryKey,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { siteBrandKitSave } from '../../generated/site/sdk.gen.ts'
import type { SiteBrandKitResource } from '../../generated/site/types.gen.ts'
import { zSiteSaveBrandKitInput } from '../../generated/site/zod.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { defineEditForm } from '../../resources/defineResource.tsx'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { withRevision } from '../../resources/resourceEditor.ts'
import type { ResourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { Route as brandKitRoute } from '../../routes/workspaces/$workspaceId/brand-kit.tsx'
import { boundsOf } from '../../utils/bounds.ts'
import { can } from '../../utils/permissions.ts'

// The contract's bounds (TypeSpec -> zod) are the single source for the inputs' range.
const RADIUS = boundsOf(zSiteSaveBrandKitInput.shape.buttonRadius)
const FOOTER = zSiteSaveBrandKitInput.shape.footerText

// A save is the whole kit: every value, an empty logo or footer meaning none. The revision is the
// session's to add (withRevision).
const zBrandKitBody = zSiteSaveBrandKitInput.omit({ revision: true })
type BrandKitValues = z.output<typeof zBrandKitBody>

function valuesOf(kit: SiteBrandKitResource): BrandKitValues {
  return {
    accentColor: kit.accentColor,
    textColor: kit.textColor,
    backgroundColor: kit.backgroundColor,
    headingFont: kit.headingFont,
    bodyFont: kit.bodyFont,
    logoUrl: kit.logoUrl,
    buttonTextColor: kit.buttonTextColor,
    buttonRadius: kit.buttonRadius,
    footerText: kit.footerText,
  }
}

// Unlike resourceFormSchema, an emptied text stays empty rather than leaving the field out: a save
// replaces every value. The fonts and the logo are trimmed.
const brandKitBody = z
  .custom<BrandKitValues>((value) => typeof value === 'object' && value !== null)
  .transform((values) => ({
    ...values,
    logoUrl: values.logoUrl.trim(),
    headingFont: values.headingFont.trim(),
    bodyFont: values.bodyFont.trim(),
  }))
  .pipe(zBrandKitBody)

const brandKitSchema: ResourceFormSchema<BrandKitValues, BrandKitValues> = {
  blank: {
    accentColor: '',
    textColor: '',
    backgroundColor: '',
    headingFont: '',
    bodyFont: '',
    logoUrl: '',
    buttonTextColor: '',
    buttonRadius: 0,
    footerText: '',
  },
  create: brandKitBody,
  update: brandKitBody,
  toValues: (kit) => ({
    accentColor: kit.accentColor ?? '',
    textColor: kit.textColor ?? '',
    backgroundColor: kit.backgroundColor ?? '',
    headingFont: kit.headingFont ?? '',
    bodyFont: kit.bodyFont ?? '',
    logoUrl: kit.logoUrl ?? '',
    buttonTextColor: kit.buttonTextColor ?? '',
    buttonRadius: kit.buttonRadius ?? 0,
    footerText: kit.footerText ?? '',
  }),
}

// Whether the person may save the kit: the form offers Save only then, as the API refuses everyone
// else (brand_kit:manage).
const CanChange = createContext(false)

function BrandKitFields({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<BrandKitValues, BrandKitValues>) {
  const { t } = useTranslation()
  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack>
        <SimpleGrid cols={{ base: 1, sm: 2 }}>
          <ColorInput
            label={t(($) => $.brandKit.accentColorLabel)}
            format="hex"
            {...form.getInputProps('accentColor')}
          />
          <ColorInput
            label={t(($) => $.brandKit.buttonTextColorLabel)}
            format="hex"
            {...form.getInputProps('buttonTextColor')}
          />
          <ColorInput
            label={t(($) => $.brandKit.textColorLabel)}
            format="hex"
            {...form.getInputProps('textColor')}
          />
          <ColorInput
            label={t(($) => $.brandKit.backgroundColorLabel)}
            format="hex"
            {...form.getInputProps('backgroundColor')}
          />
          <TextInput
            label={t(($) => $.brandKit.headingFontLabel)}
            description={t(($) => $.brandKit.fontDescription)}
            {...form.getInputProps('headingFont')}
          />
          <TextInput
            label={t(($) => $.brandKit.bodyFontLabel)}
            description={t(($) => $.brandKit.fontDescription)}
            {...form.getInputProps('bodyFont')}
          />
          <NumberInput
            label={t(($) => $.brandKit.buttonRadiusLabel)}
            min={RADIUS.min}
            max={RADIUS.max}
            allowDecimal={false}
            {...form.getInputProps('buttonRadius')}
          />
          <TextInput
            label={t(($) => $.brandKit.logoUrlLabel)}
            description={t(($) => $.brandKit.logoUrlDescription)}
            {...form.getInputProps('logoUrl')}
          />
        </SimpleGrid>
        <Textarea
          label={t(($) => $.brandKit.footerTextLabel)}
          description={t(($) => $.brandKit.footerTextDescription)}
          autosize
          minRows={2}
          maxLength={FOOTER.maxLength ?? undefined}
          {...form.getInputProps('footerText')}
        />
        {use(CanChange) && (
          <Group justify="flex-end">
            <Button type="submit" loading={isPending}>
              {t(($) => $.actions.save)}
            </Button>
          </Group>
        )}
      </Stack>
    </form>
  )
}

// The Brand kit is the Workspace's own, addressed by the Workspace alone: its edit runs the editor
// session of every edit (ADR 0119), with the path's id unused.
const BrandKitEditForm = defineEditForm({
  editorId: 'brandKitEditor',
  schema: brandKitSchema,
  Form: BrandKitFields,
  getOptions: ({ path }) => siteBrandKitGetOptions({ path: { workspaceId: path.workspaceId } }),
  update: withRevision(async ({ path, body, throwOnError }) =>
    siteBrandKitSave({ path: { workspaceId: path.workspaceId }, body, throwOnError }),
  ),
  operations: {},
  editedValues: valuesOf,
  texts: (t) => ({
    editTitle: t(($) => $.brandKit.title),
    updated: t(($) => $.brandKit.saved),
    loadErrorTitle: t(($) => $.brandKit.loadErrorTitle),
    saveErrorTitle: t(($) => $.brandKit.errorTitle),
  }),
})

// What the page shows of the stored kit under its form: the first save's hint, the recreation of
// the starter Templates once it is saved, or that the person may only read it.
function BrandKitActions({
  workspaceId,
  kit,
  canChange,
}: {
  workspaceId: string
  kit: SiteBrandKitResource
  canChange: boolean
}) {
  const { t } = useTranslation()
  const recreateMutation = useResourceMutation({
    mutation: siteBrandKitRecreateStarterTemplatesMutation(),
    invalidate: [siteTemplatesListQueryKey({ path: { workspaceId } })],
    successMessage: t(($) => $.brandKit.recreated),
    errorTitle: t(($) => $.brandKit.recreateErrorTitle),
    forbiddenMessage: t(($) => $.brandKit.forbidden),
  })

  const confirmRecreate = () => {
    modals.openConfirmModal({
      title: t(($) => $.brandKit.recreateTitle),
      children: <Text>{t(($) => $.brandKit.recreateDescription)}</Text>,
      labels: {
        cancel: t(($) => $.actions.cancel),
        confirm: t(($) => $.brandKit.recreateConfirm),
      },
      onConfirm: () => recreateMutation.mutate({ path: { workspaceId } }),
    })
  }

  if (!canChange) {
    return (
      <Text c="dimmed" size="sm">
        {t(($) => $.brandKit.readOnly)}
      </Text>
    )
  }
  if (!kit.saved) {
    return (
      <Text c="dimmed" size="sm">
        {t(($) => $.brandKit.firstSaveHint)}
      </Text>
    )
  }
  return (
    <Group justify="flex-end">
      <Button variant="default" loading={recreateMutation.isPending} onClick={confirmRecreate}>
        {t(($) => $.brandKit.recreateButton)}
      </Button>
    </Group>
  )
}

// BrandKitPage edits the Workspace's Brand kit (ADR 0040): the colors, fonts, logo, button
// style and footer the editor offers. Saving it needs brand_kit:manage; every member sees it.
// Saving it changes no message already written.
export function BrandKitPage() {
  const { t } = useTranslation()
  const { workspaceId } = brandKitRoute.useParams()

  return (
    <Stack>
      <Title order={2}>{t(($) => $.brandKit.title)}</Title>
      <Text c="dimmed" size="sm">
        {t(($) => $.brandKit.description)}
      </Text>
      <QueryBoundary fallback={<BlockSkeleton />} errorTitle={t(($) => $.brandKit.loadErrorTitle)}>
        <BrandKit workspaceId={workspaceId} />
      </QueryBoundary>
    </Stack>
  )
}

// BrandKit reads the kit and the caller's Workspace; the page's boundary shows the load and its
// failure.
function BrandKit({ workspaceId }: { workspaceId: string }) {
  const [{ data: workspaces }, { data: kit }] = useSuspenseQueries({
    queries: [siteWorkspacesListOptions(), siteBrandKitGetOptions({ path: { workspaceId } })],
  })
  const workspace = workspaces.find((w) => w.id === workspaceId)

  const queryClient = useQueryClient()
  if (!workspace) {
    return null
  }
  const canChange = can(workspace, 'brand_kit:manage')
  // Read only without brand_kit:manage: the API refuses the save of everyone else.
  return (
    <CanChange value={canChange}>
      <Fieldset variant="unstyled" disabled={!canChange}>
        <BrandKitEditForm
          workspaceId={workspaceId}
          id="brand-kit"
          loaded={kit}
          titled={false}
          // The first save also creates the starter Templates.
          onSaved={() => {
            void queryClient.invalidateQueries({
              queryKey: siteTemplatesListQueryKey({ path: { workspaceId } }),
            })
          }}
        >
          {(stored) => (
            <BrandKitActions workspaceId={workspaceId} kit={stored} canChange={canChange} />
          )}
        </BrandKitEditForm>
      </Fieldset>
    </CanChange>
  )
}
