import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../../shared/api/client'
import {
  createWikiPage,
  deleteWikiPage,
  getWikiPage,
  getWikiTree,
  moveWikiPage,
  saveTemplate,
  saveWikiPage,
} from '../../shared/api/wiki'
import { useWorkspaceOpenInstance } from '../workspace/workspaceCommands'
import { wikiStore } from './store'
import { builtinTemplates } from './templates/builtin'
import { buildDatabase, databaseTemplates, databaseTemplateTitle } from './templates/databases'
import { pageEditors, searchField, type TableLinks, type WikiActions } from './wikiContext'

interface WikiControllerOptions {
  campaignId: string
  /** enabled is false until there is a GM to read the wiki for. */
  enabled: boolean
  narrow: boolean
  /** notify tells the user something in a line over the workspace; text is an i18n key. */
  notify: (textKey: string) => void
  /** onNarrowPage and onNarrowTree switch the tabs of a narrow screen. */
  onNarrowPage?: () => void
  onNarrowTree?: () => void
  /** table is what a page may do to the game; only the table has it. */
  table?: TableLinks
}

/**
 * useWikiController reads the page tree of the campaign into the wiki store and makes the actions
 * of the wiki panels; the wiki screen and the table of the GM share it, so they work on the same data.
 */
export function useWikiController({
  campaignId,
  enabled,
  narrow,
  notify,
  onNarrowPage,
  onNarrowTree,
  table,
}: WikiControllerOptions): { actions: WikiActions; treeFailed: boolean; load: () => void } {
  const { t } = useTranslation()
  const openInstance = useWorkspaceOpenInstance()
  const [treeFailed, setTreeFailed] = useState(false)

  const reload = useCallback(async () => {
    const nodes = await getWikiTree(campaignId)
    wikiStore.getState().setTree(nodes)
    setTreeFailed(false)
  }, [campaignId])

  const load = useCallback(() => {
    reload().catch(() => {
      setTreeFailed(true)
    })
  }, [reload])

  useEffect(() => {
    if (!enabled) return
    wikiStore.getState().reset()
    let cancelled = false
    getWikiTree(campaignId).then(
      (nodes) => {
        if (cancelled) return
        wikiStore.getState().setTree(nodes)
        setTreeFailed(false)
      },
      () => {
        if (!cancelled) setTreeFailed(true)
      },
    )
    return () => {
      cancelled = true
    }
  }, [campaignId, enabled])

  // Another window may have changed the wiki: read the tree again when this one gets the focus.
  useEffect(() => {
    if (!enabled) return
    const onFocus = () => {
      reload().catch(() => undefined)
    }
    window.addEventListener('focus', onFocus)
    return () => {
      window.removeEventListener('focus', onFocus)
    }
  }, [enabled, reload])

  const actions = useMemo<WikiActions>(() => {
    const open = (id: string, opts?: { newGroup?: boolean }) => {
      wikiStore.getState().openPage(id)
      if (narrow) {
        onNarrowPage?.()
        return
      }
      openInstance({
        kind: 'wiki.page',
        key: id,
        params: { pageId: id },
        newGroup: opts?.newGroup === true,
      })
    }
    return {
      campaignId,
      table,
      reload,
      open,
      create: async (parentId, template) => {
        try {
          const body: Parameters<typeof createWikiPage>[1] = { parentId }
          if (template !== undefined && 'database' in template) {
            const tpl = databaseTemplates.find((d) => d.id === template.database)
            body.kind = 'database'
            body.title = tpl === undefined ? t('wiki.db.new.title') : databaseTemplateTitle(t, tpl)
            if (tpl !== undefined) body.icon = tpl.icon
            body.database = buildDatabase(t, tpl)
          } else if (template !== undefined && 'custom' in template) {
            body.templateId = template.custom
          } else if (template !== undefined) {
            const builtin = builtinTemplates.find((b) => b.id === template.builtin)
            if (builtin !== undefined) {
              body.icon = builtin.icon
              body.content = builtin.content(t)
            }
          }
          const page = await createWikiPage(campaignId, body)
          await reload()
          open(page.id)
        } catch {
          notify('wiki.tree.createFailed')
        }
      },
      rename: async (id, title) => {
        wikiStore.getState().retitle(id, title)
        const editor = pageEditors.get(id)
        if (editor !== undefined) {
          editor.setTitle(title)
          return
        }
        try {
          const page = await getWikiPage(id)
          await saveWikiPage(id, {
            version: page.version,
            title,
            icon: page.icon,
            coverAssetId: page.coverAssetId,
            content: page.content,
          })
        } catch {
          notify('wiki.tree.renameFailed')
          reload().catch(() => undefined)
        }
      },
      move: async (id, parentId, index) => {
        try {
          await moveWikiPage(id, parentId, index)
        } catch {
          notify('wiki.tree.moveFailed')
        }
        await reload().catch(() => undefined)
      },
      remove: async (id) => {
        try {
          await deleteWikiPage(id)
          wikiStore.getState().removePage(id)
        } catch {
          notify('wiki.tree.deleteFailed')
        }
        wikiStore.getState().bumpTools()
        await reload().catch(() => undefined)
      },
      saveAsTemplate: async (id, title) => {
        try {
          await saveTemplate(campaignId, id, title === '' ? t('wiki.untitled') : title)
          wikiStore.getState().bumpTools()
          notify('wiki.tools.templates.saved')
        } catch (err) {
          notify(
            err instanceof ApiError && err.status === 409
              ? 'wiki.tools.templates.limit'
              : 'wiki.tools.templates.saveFailed',
          )
        }
      },
      focusSearch: () => {
        searchField.pending = true
        onNarrowTree?.()
        searchField.focus?.()
      },
      notify,
    }
  }, [campaignId, narrow, notify, onNarrowPage, onNarrowTree, openInstance, reload, t, table])

  return { actions, treeFailed, load }
}
