import { ContactCsvColumn } from '../generated/site/types.gen.ts'

// The column choices of the CSV export of Contacts, grouped as the export modal shows them: every
// member of the contract's ContactCsvColumn, each in one group.
export const csvExportGroups: {
  name: 'keys' | 'attributes' | 'fields' | 'channels'
  columns: ContactCsvColumn[]
}[] = [
  {
    name: 'keys',
    columns: [ContactCsvColumn.EMAIL, ContactCsvColumn.PHONE, ContactCsvColumn.SUBJECT_ID],
  },
  {
    name: 'attributes',
    columns: [
      ContactCsvColumn.FIRST_NAME,
      ContactCsvColumn.LAST_NAME,
      ContactCsvColumn.TIME_ZONE,
      ContactCsvColumn.COUNTRY,
      ContactCsvColumn.LOCALE,
      ContactCsvColumn.BIRTHDAY,
      ContactCsvColumn.CITY,
      ContactCsvColumn.AVATAR,
      ContactCsvColumn.SOURCE,
      ContactCsvColumn.CREATED_AT,
      ContactCsvColumn.UPDATED_AT,
    ],
  },
  {
    name: 'fields',
    columns: [ContactCsvColumn.CUSTOM_FIELDS, ContactCsvColumn.TAGS],
  },
  {
    name: 'channels',
    columns: [
      ContactCsvColumn.EMAIL_CHANNEL,
      ContactCsvColumn.SMS_CHANNEL,
      ContactCsvColumn.TELEGRAM_CHANNEL,
    ],
  },
]

// Every column, in the order the modal shows them: what an export holds unless the Member
// unticks some.
export const everyCsvColumn: ContactCsvColumn[] = csvExportGroups.flatMap((group) => group.columns)
