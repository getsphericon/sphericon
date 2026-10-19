import {
  Alert,
  Button,
  Card,
  ColorInput,
  Group,
  Loader,
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

import {
  siteBrandKitGetOptions,
  siteBrandKitGetQueryKey,
  siteBrandKitRecreateStarterTemplatesMutation,
  siteBrandKitSaveMutation,
  siteTemplatesListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteBrandKitResource,
  SiteSaveBrandKitInput,
  SiteWorkspaceResource,
} from '../../generated/site/types.gen.ts'
import { canManage } from '../../hooks/useCurrentRole.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'

const MAX_RADIUS = 32
const MAX_FOOTER = 500

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
// effect; the section re-keys it when a save changes the kit.
function BrandKitForm({
  slug,
  kit,
  canChange,
}: {
  slug: string
  kit: SiteBrandKitResource
  canChange: boolean
}) {
  const { t } = useTranslation()
  const form = useForm<SiteSaveBrandKitInput>({ initialValues: valuesOf(kit) })

  const saveMutation = useResourceMutation({
    mutation: siteBrandKitSaveMutation(),
    // The first save also creates the starter Templates.
    invalidate: [
      siteBrandKitGetQueryKey({ path: { slug } }),
      siteTemplatesListQueryKey({ path: { slug } }),
    ],
    successMessage: t(($) => $.brandKit.saved),
    errorTitle: t(($) => $.brandKit.errorTitle),
    forbiddenMessage: t(($) => $.brandKit.forbidden),
  })
  const recreateMutation = useResourceMutation({
    mutation: siteBrandKitRecreateStarterTemplatesMutation(),
    invalidate: [siteTemplatesListQueryKey({ path: { slug } })],
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
      onConfirm: () => recreateMutation.mutate({ path: { slug } }),
    })
  }

  return (
    <form
      onSubmit={form.onSubmit((values) =>
        saveMutation.mutate({
          path: { slug },
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
            min={0}
            max={MAX_RADIUS}
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
          maxLength={MAX_FOOTER}
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

// BrandKitSection edits the Workspace's Brand kit (ADR 0040): the colors, fonts, logo, button
// style and footer the editor offers. Owners and admins save it; every member sees it. Saving it
// changes no message already written.
export function BrandKitSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const { slug } = workspace
  const kitQuery = useQuery(siteBrandKitGetOptions({ path: { slug } }))

  return (
    <Card withBorder>
      <Title order={4} mb="xs">
        {t(($) => $.brandKit.title)}
      </Title>
      <Text c="dimmed" size="sm" mb="sm">
        {t(($) => $.brandKit.description)}
      </Text>
      {kitQuery.isError ? (
        <Alert color="red" title={t(($) => $.brandKit.loadErrorTitle)}>
          {t(($) => $.brandKit.loadErrorTitle)}
        </Alert>
      ) : null}
      {kitQuery.isLoading ? <Loader size="sm" /> : null}
      {kitQuery.data ? (
        <BrandKitForm
          key={kitQuery.data.updatedAt ?? 'unsaved'}
          slug={slug}
          kit={kitQuery.data}
          canChange={canManage(workspace.role)}
        />
      ) : null}
    </Card>
  )
}
