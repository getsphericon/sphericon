import {
  Alert,
  Button,
  ColorInput,
  Group,
  NumberInput,
  SimpleGrid,
  Stack,
  Text,
  Textarea,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { modals } from '@mantine/modals'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { BlockSkeleton } from '../../components/Skeletons.tsx'
import {
  siteBrandKitGetOptions,
  siteBrandKitGetQueryKey,
  siteBrandKitRecreateStarterTemplatesMutation,
  siteBrandKitSaveMutation,
  siteTemplatesListQueryKey,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteBrandKitResource, SiteSaveBrandKitInput } from '../../generated/site/types.gen.ts'
import { zSiteSaveBrandKitInput } from '../../generated/site/zod.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as brandKitRoute } from '../../routes/workspaces/$workspaceId/brand-kit.tsx'
import { can } from '../../utils/permissions.ts'

// The contract's bounds (TypeSpec -> zod) are the single source for the inputs' range.
const RADIUS = zSiteSaveBrandKitInput.shape.buttonRadius
const FOOTER = zSiteSaveBrandKitInput.shape.footerText

function valuesOf(kit: SiteBrandKitResource): SiteSaveBrandKitInput {
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

// BrandKitForm is split out so the form seeds its values from the loaded Brand kit without an
// effect; the page re-keys it when a save changes the kit.
function BrandKitForm({
  workspaceId,
  kit,
  canChange,
}: {
  workspaceId: string
  kit: SiteBrandKitResource
  canChange: boolean
}) {
  const { t } = useTranslation()
  const form = useForm<SiteSaveBrandKitInput>({ initialValues: valuesOf(kit) })

  const saveMutation = useResourceMutation({
    mutation: siteBrandKitSaveMutation(),
    // The first save also creates the starter Templates.
    invalidate: [
      siteBrandKitGetQueryKey({ path: { workspaceId } }),
      siteTemplatesListQueryKey({ path: { workspaceId } }),
    ],
    successMessage: t(($) => $.brandKit.saved),
    errorTitle: t(($) => $.brandKit.errorTitle),
    forbiddenMessage: t(($) => $.brandKit.forbidden),
  })
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

  return (
    <form
      onSubmit={form.onSubmit((values) =>
        saveMutation.mutate({
          path: { workspaceId },
          body: {
            ...values,
            logoUrl: values.logoUrl.trim(),
            headingFont: values.headingFont.trim(),
            bodyFont: values.bodyFont.trim(),
          },
        }),
      )}
    >
      <Stack>
        <SimpleGrid cols={{ base: 1, sm: 2 }}>
          <ColorInput
            label={t(($) => $.brandKit.accentColorLabel)}
            format="hex"
            disabled={!canChange}
            {...form.getInputProps('accentColor')}
          />
          <ColorInput
            label={t(($) => $.brandKit.buttonTextColorLabel)}
            format="hex"
            disabled={!canChange}
            {...form.getInputProps('buttonTextColor')}
          />
          <ColorInput
            label={t(($) => $.brandKit.textColorLabel)}
            format="hex"
            disabled={!canChange}
            {...form.getInputProps('textColor')}
          />
          <ColorInput
            label={t(($) => $.brandKit.backgroundColorLabel)}
            format="hex"
            disabled={!canChange}
            {...form.getInputProps('backgroundColor')}
          />
          <TextInput
            label={t(($) => $.brandKit.headingFontLabel)}
            description={t(($) => $.brandKit.fontDescription)}
            disabled={!canChange}
            {...form.getInputProps('headingFont')}
          />
          <TextInput
            label={t(($) => $.brandKit.bodyFontLabel)}
            description={t(($) => $.brandKit.fontDescription)}
            disabled={!canChange}
            {...form.getInputProps('bodyFont')}
          />
          <NumberInput
            label={t(($) => $.brandKit.buttonRadiusLabel)}
            min={RADIUS.minValue ?? 0}
            max={RADIUS.maxValue ?? 0}
            allowDecimal={false}
            disabled={!canChange}
            {...form.getInputProps('buttonRadius')}
          />
          <TextInput
            label={t(($) => $.brandKit.logoUrlLabel)}
            description={t(($) => $.brandKit.logoUrlDescription)}
            disabled={!canChange}
            {...form.getInputProps('logoUrl')}
          />
        </SimpleGrid>
        <Textarea
          label={t(($) => $.brandKit.footerTextLabel)}
          description={t(($) => $.brandKit.footerTextDescription)}
          autosize
          minRows={2}
          maxLength={FOOTER.maxLength ?? undefined}
          disabled={!canChange}
          {...form.getInputProps('footerText')}
        />
        {canChange ? (
          <>
            {kit.saved ? null : (
              <Text c="dimmed" size="sm">
                {t(($) => $.brandKit.firstSaveHint)}
              </Text>
            )}
            <Group justify="flex-end">
              {kit.saved ? (
                <Button
                  variant="default"
                  loading={recreateMutation.isPending}
                  onClick={confirmRecreate}
                >
                  {t(($) => $.brandKit.recreateButton)}
                </Button>
              ) : null}
              <Button type="submit" loading={saveMutation.isPending}>
                {t(($) => $.actions.save)}
              </Button>
            </Group>
          </>
        ) : (
          <Text c="dimmed" size="sm">
            {t(($) => $.brandKit.readOnly)}
          </Text>
        )}
      </Stack>
    </form>
  )
}

// BrandKitPage edits the Workspace's Brand kit (ADR 0040): the colors, fonts, logo, button
// style and footer the editor offers. Saving it needs brand_kit:manage; every member sees it.
// Saving it changes no message already written.
export function BrandKitPage() {
  const { t } = useTranslation()
  const { workspaceId } = brandKitRoute.useParams()
  const workspacesQuery = useQuery(siteWorkspacesListOptions())
  const kitQuery = useQuery(siteBrandKitGetOptions({ path: { workspaceId } }))
  const workspace = workspacesQuery.data?.find((w) => w.id === workspaceId)

  return (
    <Stack>
      <Title order={2}>{t(($) => $.brandKit.title)}</Title>
      <Text c="dimmed" size="sm">
        {t(($) => $.brandKit.description)}
      </Text>
      {kitQuery.isError || workspacesQuery.isError ? (
        <Alert color="red" title={t(($) => $.brandKit.loadErrorTitle)}>
          {t(($) => $.brandKit.loadErrorTitle)}
        </Alert>
      ) : null}
      {kitQuery.isLoading || workspacesQuery.isLoading ? <BlockSkeleton /> : null}
      {kitQuery.data && workspace ? (
        <BrandKitForm
          key={kitQuery.data.updatedAt ?? 'unsaved'}
          workspaceId={workspaceId}
          kit={kitQuery.data}
          canChange={can(workspace, 'brand_kit:manage')}
        />
      ) : null}
    </Stack>
  )
}
