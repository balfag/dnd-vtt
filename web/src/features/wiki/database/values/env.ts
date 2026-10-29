import { createContext, useContext } from 'react'
import { errorKey } from '../../../../shared/api/errors'
import { setRecordProps, type DbRecord, type Property } from '../../../../shared/api/wiki'

/** ValueEnv is what the editors of values need from the database around them. */
export interface ValueEnv {
  campaignId: string
  /** createOption adds a variant to a select and gives the id of the new one. */
  createOption: (prop: Property, name: string) => Promise<string>
  /** relations holds the records of the target databases of the relations, by database id and record id; a database not read yet is absent. */
  relations: ReadonlyMap<string, ReadonlyMap<string, DbRecord>>
  /** openRecord shows a record as a page. */
  openRecord: (id: string) => void
}

export const ValueEnvContext = createContext<ValueEnv | null>(null)

export function useValueEnv(): ValueEnv {
  const env = useContext(ValueEnvContext)
  if (env === null) throw new Error('useValueEnv: no database around')
  return env
}

/**
 * saveValue stores one value of a record at once. The caller has already shown the new value;
 * when the server refuses it, rollback puts the old one back and the result is the i18n key of the reason.
 */
export async function saveValue(
  recordId: string,
  propId: string,
  next: unknown,
  rollback: () => void,
): Promise<string | null> {
  try {
    await setRecordProps(recordId, { [propId]: next ?? null })
    return null
  } catch (err) {
    rollback()
    return errorKey(err)
  }
}
