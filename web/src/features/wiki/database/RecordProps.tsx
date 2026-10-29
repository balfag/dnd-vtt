import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Property, WikiPage } from '../../../shared/api/wiki'
import { useWikiActions } from '../wikiContext'
import { addOption } from './options'
import { rememberGameLabel, withKey } from './model'
import { useDatabase } from './useDatabase'
import { saveValue, ValueEnvContext, type ValueEnv } from './values/env'
import { useRelations } from './values/useRelations'
import { ValueCell } from './values/ValueCell'
import './database.css'

/** RecordProps shows the properties of a record under its title: two columns "name | value" with the same editors as the table. */
export function RecordProps({ page }: { page: WikiPage }) {
  const { t } = useTranslation()
  const actions = useWikiActions()
  const databaseId = page.parentId ?? ''
  const { load, update, version } = useDatabase(databaseId)
  const [values, setValues] = useState<Record<string, unknown>>(page.props)
  const [editing, setEditing] = useState<string | null>(null)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const properties = useMemo<Property[]>(
    () => (load.status === 'ready' ? load.db.properties : []),
    [load],
  )
  const relations = useRelations(properties, version)

  // The names of the scenes and tokens the page already knows are those the cells can show.
  useEffect(() => {
    for (const [key, ref] of Object.entries(page.refs)) {
      const at = key.indexOf(':')
      if (ref.exists && at > 0) rememberGameLabel(key.slice(0, at), key.slice(at + 1), ref.label)
    }
  }, [page.refs])

  const createOption = useCallback(
    (prop: Property, name: string) =>
      addOption(prop, name, (saved) => {
        update((d) => ({
          ...d,
          properties: d.properties.map((p) => (p.id === saved.id ? saved : p)),
        }))
      }),
    [update],
  )

  const env = useMemo<ValueEnv>(
    () => ({
      campaignId: page.campaignId,
      createOption,
      relations,
      openRecord: (id) => {
        actions.open(id)
      },
    }),
    [page.campaignId, createOption, relations, actions],
  )

  function change(prop: Property, next: unknown) {
    const before = values[prop.id]
    const put = (value: unknown) => {
      setValues((now) => withKey(now, prop.id, value))
    }
    put(next)
    void saveValue(page.id, prop.id, next, () => {
      put(before)
    }).then((key) => {
      setErrors((now) => withKey(now, prop.id, key === null ? null : t(key)))
    })
  }

  if (load.status !== 'ready' || properties.length === 0) return null

  return (
    <ValueEnvContext value={env}>
      <dl className="recprops" aria-label={t('wiki.db.record.props')}>
        {properties.map((prop) => (
          <div key={prop.id} className="recprops__row">
            <dt className="recprops__name">{prop.name}</dt>
            <dd className="recprops__value">
              <ValueCell
                prop={prop}
                value={values[prop.id]}
                editing={editing === prop.id}
                error={errors[prop.id]}
                onClear={() => {
                  setErrors((now) => withKey<string>(now, prop.id, null))
                }}
                onEdit={() => {
                  setEditing(prop.id)
                }}
                onEnd={() => {
                  setEditing(null)
                }}
                onChange={(next) => {
                  change(prop, next)
                }}
              />
            </dd>
          </div>
        ))}
      </dl>
    </ValueEnvContext>
  )
}
