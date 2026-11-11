import { Checkbox } from '@mantine/core'
import type { CheckboxProps } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { SignupItemKind } from '../generated/site/types.gen.ts'
import type { SiteSignupItem } from '../generated/site/types.gen.ts'
import { Route as legalDocumentRoute } from '../routes/legal.$key.tsx'
import { signupItemField } from '../utils/signupItems.ts'
import { AnchorLink } from './RouterLink.tsx'

interface SignupItemFieldsProps {
  // The Sign-up items in effect, in the order of the form.
  items: readonly SiteSignupItem[]
  // The form's props of an item's box, at the item's field (signupItemField): whether the person
  // ticked it, and the refusal the form shows under it.
  getInputProps: (path: string, options: { type: 'checkbox' }) => CheckboxProps
}

// The Sign-up items a form that sets up an account asks (ADR 0062): each required acceptance and
// optional consent as an unticked checkbox linking to its Legal document, each notice as a link.
// The boxes are fields of the form, so a refusal of one shows and takes the focus like any field's.
export function SignupItemFields({ items, getInputProps }: SignupItemFieldsProps) {
  const { t } = useTranslation()
  return items.map((item) =>
    item.kind === SignupItemKind.NOTICE ? (
      <AnchorLink
        key={item.key}
        to={legalDocumentRoute.to}
        params={{ key: item.key }}
        target="_blank"
        size="sm"
      >
        {item.title}
      </AnchorLink>
    ) : (
      <SignupItemBox
        key={item.key}
        label={item.title}
        inputProps={getInputProps(signupItemField(item.key), { type: 'checkbox' })}
        description={
          <AnchorLink
            to={legalDocumentRoute.to}
            params={{ key: item.key }}
            target="_blank"
            size="xs"
          >
            {t(($) => $.signUp.readItem, { title: item.title })}
          </AnchorLink>
        }
      />
    ),
  )
}

// SignupItemBox is an item's box, unticked until the person ticks it: the form holds no value for
// an item it has not seen ticked.
function SignupItemBox({ inputProps, ...props }: CheckboxProps & { inputProps: CheckboxProps }) {
  return <Checkbox {...props} {...inputProps} checked={inputProps.checked === true} />
}
