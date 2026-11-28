import { describe, expect, it } from 'vitest'

import { ContactCsvColumn } from '../generated/site/types.gen.ts'
import { everyCsvColumn } from './contactCsvExport.ts'

describe('the column choices of the CSV export', () => {
  it('offer every column of the contract once', () => {
    expect(everyCsvColumn.toSorted()).toEqual(Object.values(ContactCsvColumn).toSorted())
  })
})
