import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../../../shared/api/client'
import { errorKey } from '../../../shared/api/errors'
import {
  addProperty,
  addView,
  createWikiPage,
  deleteProperty,
  deleteView,
  getWikiPage,
  listTags,
  saveWikiPage,
  updateProperty,
  updateView,
  type DbRecord,
  type Filter,
  type Property,
  type PropertyOptions,
  type PropertyType,
  type SortKey,
  type View,
  type ViewConfig,
  type ViewKind,
  type WikiPage,
} from '../../../shared/api/wiki'
import { Button } from '../../../ui/Button'
import { ErrorNote } from '../../../ui/ErrorNote'
import { wikiStore } from '../store'
import { useWikiActions } from '../wikiContext'
import { CardsView } from './CardsView'
import { filterText, sortText } from './describe'
import { FilterEditor } from './FilterEditor'
import { applyView } from './filter'
import { withKey } from './model'
import { addOption } from './options'
import { Popover } from './Popover'
import { PropertiesMenu } from './PropertiesMenu'
import { PropertyDialog } from './PropertyDialog'
import { SortEditor } from './SortEditor'
import { TableView } from './TableView'
import { useDatabase } from './useDatabase'
import { ValueEnvContext, saveValue, type ValueEnv } from './values/env'
import { useRelations } from './values/useRelations'
import { ViewTabs } from './ViewTabs'
import './database.css'

const SAVE_DELAY_MS = 500
const emptyConfig: ViewConfig = { filters: [], sorts: [], visible: [], widths: {} }
const MAX_TITLE = 200

type PopKind = 'filter' | 'sort' | 'props'

/** DatabasePanel is a database of the wiki in place of a page: views, filters, properties, and the records. */
export function DatabasePanel({ page }: { page: WikiPage }) {
  const { t } = useTranslation()
  const actions = useWikiActions()
  const { load, update, reload, version } = useDatabase(page.id)

  if (load.status === 'loading') return <p className="page__note">{t('wiki.loading')}</p>
  if (load.status === 'failed') {
    return (
      <div className="page__note">
        <ErrorNote>{t(load.gone ? 'wiki.page.gone' : 'wiki.db.loadFailed')}</ErrorNote>
        {!load.gone && (
          <Button
            variant="secondary"
            fullWidth={false}
            onClick={() => {
              reload(false)
            }}
          >
            {t('wiki.retry')}
          </Button>
        )}
      </div>
    )
  }
  return (
    <DatabaseView
      page={page}
      db={load.db}
      update={update}
      reload={reload}
      version={version}
      notify={actions.notify}
    />
  )
}

interface DatabaseViewProps {
  page: WikiPage
  db: { properties: Property[]; views: View[]; records: DbRecord[] }
  update: ReturnType<typeof useDatabase>['update']
  reload: ReturnType<typeof useDatabase>['reload']
  version: number
  notify: (textKey: string) => void
}

function DatabaseView({ page, db, update, reload, version, notify }: DatabaseViewProps) {
  const { t } = useTranslation()
  const actions = useWikiActions()
  const { properties, views, records } = db
  const [selected, setSelected] = useState(views[0]?.id ?? '')
  const [title, setTitle] = useState(page.title)
  const [pop, setPop] = useState<{ kind: PopKind; anchor: HTMLElement } | null>(null)
  const [addingProperty, setAddingProperty] = useState(false)
  const [errors, setErrors] = useState<Record<string, Record<string, string>>>({})
  const [newRecord, setNewRecord] = useState<string | null>(null)
  const [tagNames, setTagNames] = useState<Map<string, string>>(new Map())
  const pageVersion = useRef(page.version)
  const savedTitle = useRef(page.title)
  const viewsRef = useRef(views)
  const timers = useRef(new Map<string, ReturnType<typeof setTimeout>>())
  const relations = useRelations(properties, version)

  useEffect(() => {
    viewsRef.current = views
  }, [views])

  const view = views.find((v) => v.id === selected) ?? views[0]
  const config = view?.config ?? emptyConfig

  useEffect(() => {
    let stale = false
    listTags(page.campaignId).then(
      (list) => {
        if (!stale) setTagNames(new Map(list.map((x) => [x.id, x.name])))
      },
      () => undefined,
    )
    return () => {
      stale = true
    }
  }, [page.campaignId, version])

  // A view being edited is saved after a pause; whatever is waiting goes out when the panel closes.
  const send = useCallback(
    (viewId: string) => {
      timers.current.delete(viewId)
      const v = viewsRef.current.find((x) => x.id === viewId)
      if (v === undefined) return
      updateView(viewId, { config: v.config }).catch(() => {
        notify('wiki.db.view.saveFailed')
        reload()
      })
    },
    [notify, reload],
  )
  useEffect(() => {
    const pending = timers.current
    return () => {
      for (const [id, timer] of pending) {
        clearTimeout(timer)
        send(id)
      }
      pending.clear()
    }
  }, [send])

  const changeConfig = useCallback(
    (v: View, next: ViewConfig, delay: number) => {
      update((d) => ({
        ...d,
        views: d.views.map((x) => (x.id === v.id ? { ...x, config: next } : x)),
      }))
      viewsRef.current = viewsRef.current.map((x) => (x.id === v.id ? { ...x, config: next } : x))
      clearTimeout(timers.current.get(v.id))
      timers.current.set(
        v.id,
        setTimeout(() => {
          send(v.id)
        }, delay),
      )
    },
    [update, send],
  )

  const columns = useMemo(
    () => config.visible.flatMap((id) => properties.find((p) => p.id === id) ?? []),
    [config.visible, properties],
  )
  const shown = useMemo(
    () => applyView(records, properties, config, tagNames),
    [records, properties, config, tagNames],
  )

  // ---- the title of the database

  async function saveTitle(next: string) {
    if (next === savedTitle.current) return
    savedTitle.current = next
    wikiStore.getState().retitle(page.id, next)
    const body = (v: number) => ({
      version: v,
      title: next,
      icon: page.icon,
      coverAssetId: page.coverAssetId,
      content: page.content,
    })
    try {
      let saved
      try {
        saved = await saveWikiPage(page.id, body(pageVersion.current))
      } catch (err) {
        if (!(err instanceof ApiError) || err.status !== 409) throw err
        const fresh = await getWikiPage(page.id)
        saved = await saveWikiPage(page.id, body(fresh.version))
      }
      pageVersion.current = saved.version
    } catch {
      notify('wiki.tree.renameFailed')
    }
  }

  // ---- values of the records

  const setError = useCallback((recordId: string, propId: string, text: string | null) => {
    setErrors((now) => {
      if (text === null && now[recordId]?.[propId] === undefined) return now
      return { ...now, [recordId]: withKey(now[recordId] ?? {}, propId, text) }
    })
  }, [])

  const changeValue = useCallback(
    (record: DbRecord, prop: Property, next: unknown) => {
      const before = record.props[prop.id]
      const put = (value: unknown) => {
        update((d) => ({
          ...d,
          records: d.records.map((r) => {
            if (r.id !== record.id) return r
            return { ...r, props: withKey(r.props, prop.id, value) }
          }),
        }))
      }
      put(next)
      saveValue(record.id, prop.id, next, () => {
        put(before)
      }).then(
        (key) => {
          setError(record.id, prop.id, key === null ? null : t(key))
        },
        () => undefined,
      )
    },
    [update, setError, t],
  )

  const clearError = useCallback(
    (recordId: string, propId: string) => {
      setError(recordId, propId, null)
    },
    [setError],
  )

  const renameRecord = useCallback(
    (record: DbRecord, next: string) => {
      update((d) => ({
        ...d,
        records: d.records.map((r) => (r.id === record.id ? { ...r, title: next } : r)),
      }))
      void actions.rename(record.id, next)
    },
    [update, actions],
  )

  const addRecord = useCallback(async () => {
    try {
      const created = await createWikiPage(page.campaignId, { parentId: page.id })
      update((d) => ({
        ...d,
        records: [
          ...d.records,
          {
            id: created.id,
            title: created.title,
            icon: created.icon,
            coverAssetId: created.coverAssetId,
            props: {},
            tags: [],
            createdAt: created.createdAt,
            updatedAt: created.updatedAt,
          },
        ],
      }))
      setNewRecord(created.id)
      actions.reload().catch(() => undefined)
    } catch {
      notify('wiki.db.table.createFailed')
    }
  }, [page.campaignId, page.id, update, actions, notify])

  const deleteRecord = useCallback(
    (id: string) => {
      update((d) => ({ ...d, records: d.records.filter((r) => r.id !== id) }))
      void actions.remove(id).then(() => {
        reload()
      })
    },
    [update, actions, reload],
  )

  const openRecord = useCallback(
    (id: string) => {
      actions.open(id)
    },
    [actions],
  )

  // ---- properties

  const failed = useCallback(
    (err: unknown, fallback: string) => {
      notify(err instanceof ApiError && err.status === 400 ? 'wiki.db.property.refused' : fallback)
    },
    [notify],
  )

  const patchProperty = useCallback(
    (next: Property) => {
      update((d) => ({
        ...d,
        properties: d.properties.map((p) => (p.id === next.id ? next : p)),
      }))
    },
    [update],
  )

  const createOption = useCallback(
    (prop: Property, name: string) => addOption(prop, name, patchProperty),
    [patchProperty],
  )

  const env = useMemo<ValueEnv>(
    () => ({ campaignId: page.campaignId, createOption, relations, openRecord }),
    [page.campaignId, createOption, relations, openRecord],
  )

  async function onAddProperty(p: {
    name: string
    type: PropertyType
    options: PropertyOptions
  }): Promise<string | null> {
    try {
      const made = await addProperty(page.id, p)
      update((d) => ({ ...d, properties: [...d.properties, made] }))
      if (view !== undefined) {
        changeConfig(view, { ...config, visible: [...config.visible, made.id] }, 0)
      }
      return null
    } catch (err) {
      return errorKey(err)
    }
  }

  async function onRenameProperty(prop: Property, name: string): Promise<boolean> {
    try {
      patchProperty(await updateProperty(prop.id, { name }))
      return true
    } catch (err) {
      failed(err, 'wiki.db.property.saveFailed')
      return false
    }
  }

  async function onMoveProperty(prop: Property, delta: -1 | 1) {
    const from = properties.findIndex((p) => p.id === prop.id)
    const to = from + delta
    if (from < 0 || to < 0 || to >= properties.length) return
    const order = properties.slice()
    order.splice(to, 0, ...order.splice(from, 1))
    update((d) => ({ ...d, properties: order.map((p, i) => ({ ...p, position: i })) }))
    if (view !== undefined) {
      const on = new Set(config.visible)
      changeConfig(
        view,
        { ...config, visible: order.map((p) => p.id).filter((id) => on.has(id)) },
        0,
      )
    }
    try {
      await updateProperty(prop.id, { position: to })
    } catch (err) {
      failed(err, 'wiki.db.property.saveFailed')
      reload()
    }
  }

  async function onOptions(prop: Property, options: PropertyOptions, cleared: boolean) {
    try {
      patchProperty(await updateProperty(prop.id, { options }))
      if (cleared) reload()
    } catch (err) {
      failed(err, 'wiki.db.property.saveFailed')
      reload()
    }
  }

  async function onDeleteProperty(prop: Property) {
    try {
      await deleteProperty(prop.id)
      reload()
    } catch (err) {
      failed(err, 'wiki.db.property.saveFailed')
    }
  }

  // ---- views

  async function onAddView(name: string, kind: ViewKind) {
    try {
      const made = await addView(page.id, { name, kind })
      update((d) => ({ ...d, views: [...d.views, made] }))
      setSelected(made.id)
    } catch {
      notify('wiki.db.view.saveFailed')
    }
  }

  async function onRenameView(v: View, name: string) {
    update((d) => ({ ...d, views: d.views.map((x) => (x.id === v.id ? { ...x, name } : x)) }))
    try {
      await updateView(v.id, { name })
    } catch {
      notify('wiki.db.view.saveFailed')
      reload()
    }
  }

  async function onDeleteView(v: View) {
    try {
      await deleteView(v.id)
      const rest = views.filter((x) => x.id !== v.id)
      update((d) => ({ ...d, views: rest }))
      setSelected(rest[0]?.id ?? '')
    } catch {
      notify('wiki.db.view.saveFailed')
    }
  }

  // ---- parts of the screen

  function openPop(kind: PopKind, anchor: HTMLElement) {
    setPop((now) => (now?.kind === kind ? null : { kind, anchor }))
  }

  const filtered = config.filters.length > 0 || config.sorts.length > 0
  const name = title === '' ? t('wiki.untitled') : title

  const toolbar = (
    <div className="db__tools">
      {(
        [
          ['filter', t('wiki.db.filter.button'), config.filters.length],
          ['sort', t('wiki.db.sort.button'), config.sorts.length],
          ['props', t('wiki.db.property.button'), 0],
        ] as const
      ).map(([kind, label, count]) => (
        <button
          key={kind}
          type="button"
          className="db__tool"
          aria-haspopup="dialog"
          aria-expanded={pop?.kind === kind}
          onClick={(e) => {
            openPop(kind, e.currentTarget)
          }}
        >
          {label}
          {count > 0 && <span className="db__badge">{count}</span>}
        </button>
      ))}
    </div>
  )

  return (
    <ValueEnvContext value={env}>
      <article className="db" aria-label={name}>
        <header className="db__head">
          <span className="db__icon" aria-hidden="true">
            {page.icon !== '' ? page.icon : '▦'}
          </span>
          <input
            className="db__title"
            aria-label={t('wiki.page.title')}
            placeholder={t('wiki.page.titlePlaceholder')}
            value={title}
            maxLength={MAX_TITLE}
            onChange={(e) => {
              setTitle(e.target.value)
            }}
            onBlur={() => {
              void saveTitle(title)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') e.currentTarget.blur()
            }}
          />
          <span className="db__count">{t('wiki.tree.records', { count: records.length })}</span>
        </header>

        <div className="db__bar">
          <ViewTabs
            views={views}
            selected={view?.id ?? ''}
            onSelect={setSelected}
            onAdd={(n, k) => void onAddView(n, k)}
            onRename={(v, n) => void onRenameView(v, n)}
            onDelete={(v) => void onDeleteView(v)}
          />
          {toolbar}
        </div>

        {filtered && (
          <div className="db__conditions" role="group" aria-label={t('wiki.db.conditions')}>
            {config.filters.map((f, i) => (
              <button
                key={`f${String(i)}`}
                type="button"
                className="db__chip"
                onClick={(e) => {
                  openPop('filter', e.currentTarget)
                }}
              >
                {filterText(t, f, properties, tagNames)}
              </button>
            ))}
            {config.sorts.map((s, i) => (
              <button
                key={`s${String(i)}`}
                type="button"
                className="db__chip"
                onClick={(e) => {
                  openPop('sort', e.currentTarget)
                }}
              >
                {sortText(t, s, properties)}
              </button>
            ))}
            <span className="db__shown" role="status">
              {t('wiki.db.shown', { shown: shown.length, total: records.length })}
            </span>
          </div>
        )}

        <div className="db__scroll">
          {view?.kind === 'cards' ? (
            <CardsView columns={columns} records={shown} onOpen={openRecord} />
          ) : (
            <TableView
              columns={columns}
              records={shown}
              widths={config.widths}
              onWidth={(key, px) => {
                if (view !== undefined) {
                  changeConfig(
                    view,
                    { ...config, widths: { ...config.widths, [key]: px } },
                    SAVE_DELAY_MS,
                  )
                }
              }}
              errors={errors}
              onClearError={clearError}
              onChange={changeValue}
              onTitle={renameRecord}
              onOpen={openRecord}
              onDelete={deleteRecord}
              newRecord={newRecord}
              onNewRecordSeen={() => {
                setNewRecord(null)
              }}
            />
          )}
          {records.length === 0 && <p className="db__empty">{t('wiki.db.empty')}</p>}
          {records.length > 0 && shown.length === 0 && (
            <p className="db__empty">{t('wiki.db.noMatch')}</p>
          )}
          <button
            type="button"
            className="db__new"
            onClick={() => {
              void addRecord()
            }}
          >
            {t('wiki.db.table.new')}
          </button>
        </div>

        {pop !== null && view !== undefined && (
          <Popover
            anchor={pop.anchor}
            label={t(`wiki.db.${pop.kind}.title`)}
            align="start"
            onClose={() => {
              setPop(null)
            }}
          >
            {pop.kind === 'filter' && (
              <FilterEditor
                key={view.id}
                campaignId={page.campaignId}
                properties={properties}
                filters={config.filters}
                onChange={(filters: Filter[]) => {
                  changeConfig(view, { ...config, filters }, SAVE_DELAY_MS)
                }}
              />
            )}
            {pop.kind === 'sort' && (
              <SortEditor
                properties={properties}
                sorts={config.sorts}
                onChange={(sorts: SortKey[]) => {
                  changeConfig(view, { ...config, sorts }, 0)
                }}
              />
            )}
            {pop.kind === 'props' && (
              <PropertiesMenu
                properties={properties}
                visible={config.visible}
                onVisible={(visible) => {
                  changeConfig(view, { ...config, visible }, 0)
                }}
                onMove={(p, d) => void onMoveProperty(p, d)}
                onRename={onRenameProperty}
                onOptions={(p, o, c) => void onOptions(p, o, c)}
                onDelete={(p) => void onDeleteProperty(p)}
                onAdd={() => {
                  setAddingProperty(true)
                }}
              />
            )}
          </Popover>
        )}
        {addingProperty && (
          <PropertyDialog
            open
            properties={properties}
            onClose={() => {
              setAddingProperty(false)
            }}
            onAdd={onAddProperty}
          />
        )}
      </article>
    </ValueEnvContext>
  )
}
