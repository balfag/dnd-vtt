import { useEffect, useMemo, useState } from 'react'
import { getDatabase, type DbRecord, type Property } from '../../../../shared/api/wiki'

type Relations = ReadonlyMap<string, ReadonlyMap<string, DbRecord>>

const none: Relations = new Map()

/** useRelations reads the records of every database the relation properties point at; a database that fails stays absent. */
export function useRelations(properties: Property[], reloadKey: number): Relations {
  const [loaded, setLoaded] = useState<Relations>(none)
  const targets = useMemo(
    () =>
      [
        ...new Set(
          properties
            .filter((p) => p.type === 'relation')
            .map((p) => p.options.targetDatabaseId)
            .filter((id): id is string => id !== undefined),
        ),
      ].sort(),
    [properties],
  )
  const wanted = targets.join(',')

  useEffect(() => {
    let stale = false
    const ids = wanted === '' ? [] : wanted.split(',')
    for (const id of ids) {
      getDatabase(id).then(
        (db) => {
          if (stale) return
          setLoaded((now) => new Map(now).set(id, new Map(db.records.map((r) => [r.id, r]))))
        },
        () => undefined,
      )
    }
    return () => {
      stale = true
    }
  }, [wanted, reloadKey])

  return loaded
}
