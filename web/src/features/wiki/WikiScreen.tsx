import type { DockviewApi } from 'dockview-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { navigate } from '../../app/router'
import { useSession } from '../../app/session'
import { ApiError } from '../../shared/api/client'
import { listCampaigns, type Campaign } from '../../shared/api/campaigns'
import {
  createWikiPage,
  deleteWikiPage,
  getWikiPage,
  getWikiTree,
  moveWikiPage,
  saveTemplate,
  saveWikiPage,
} from '../../shared/api/wiki'
import { AuthLayout } from '../../ui/AuthLayout'
import { Button } from '../../ui/Button'
import { ErrorNote } from '../../ui/ErrorNote'
import { Link } from '../../ui/Link'
import { useNarrow } from '../table/useNarrow'
import { layoutKey } from '../workspace/layout'
import { panelOptions, type PanelDef } from '../workspace/panels'
import { Workspace } from '../workspace/Workspace'
import { useWorkspaceOpenInstance, useWorkspaceReset } from '../workspace/workspaceCommands'
import { PagePanel } from './page/PagePanel'
import { pageIdOfPanel, pagePanelTitle, PAGE_PANEL_PREFIX } from './panelIds'
import { wikiStore, useWiki } from './store'
import { TreePanel } from './tree/TreePanel'
import { builtinTemplates } from './templates/builtin'
import { pageEditors, searchField, WikiContext, type WikiActions } from './wikiContext'
import './wiki.css'

/** TREE_WIDTH_REM is the width of the tree in the default layout. */
const TREE_WIDTH_REM = 18
const NOTICE_MS = 5000

type Narrow = 'tree' | 'page'

/** WikiScreen is the GM's notebook: /wiki/<campaignId>, the page tree and the pages in a workspace. */
export default function WikiScreen({ campaignId }: { campaignId: string }) {
  const { t } = useTranslation()
  const { state } = useSession()
  const narrow = useNarrow()
  const resetLayout = useWorkspaceReset()
  const openInstance = useWorkspaceOpenInstance()
  const tree = useWiki((s) => s.tree)
  const active = useWiki((s) => s.active)
  const [campaign, setCampaign] = useState<Campaign | 'missing' | null>(null)
  const [treeFailed, setTreeFailed] = useState(false)
  const [notice, setNotice] = useState<string | null>(null)
  const [narrowTab, setNarrowTab] = useState<Narrow>('tree')

  const anonymous = state.status === 'anonymous'
  const isGm = state.status === 'signedIn' && state.me.isGm
  const accountId = state.status === 'signedIn' ? state.me.accountId : ''

  useEffect(() => {
    if (anonymous) navigate('/login', { replace: true })
    else if (state.status === 'signedIn' && !state.me.isGm) navigate('/', { replace: true })
  }, [anonymous, state])

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
    if (!isGm) return
    wikiStore.getState().reset()
    let cancelled = false
    listCampaigns().then(
      (list) => {
        if (cancelled) return
        setCampaign(list.find((c) => c.id === campaignId) ?? 'missing')
      },
      () => {
        if (!cancelled) setCampaign('missing')
      },
    )
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
  }, [campaignId, isGm])

  // Another window may have changed the wiki: read the tree again when this one gets the focus.
  useEffect(() => {
    if (!isGm) return
    const onFocus = () => {
      reload().catch(() => undefined)
    }
    window.addEventListener('focus', onFocus)
    return () => {
      window.removeEventListener('focus', onFocus)
    }
  }, [isGm, reload])

  const noticeTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const notify = useCallback((textKey: string) => {
    setNotice(textKey)
    clearTimeout(noticeTimer.current)
    noticeTimer.current = setTimeout(() => {
      setNotice(null)
    }, NOTICE_MS)
  }, [])
  useEffect(
    () => () => {
      clearTimeout(noticeTimer.current)
    },
    [],
  )

  const actions = useMemo<WikiActions>(() => {
    const open = (id: string, opts?: { newGroup?: boolean }) => {
      wikiStore.getState().openPage(id)
      if (narrow) {
        setNarrowTab('page')
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
      reload,
      open,
      create: async (parentId, template) => {
        try {
          const body: Parameters<typeof createWikiPage>[1] = { parentId }
          if (template !== undefined && 'custom' in template) {
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
        setNarrowTab('tree')
        searchField.focus?.()
      },
      notify,
    }
  }, [campaignId, narrow, notify, openInstance, reload, t])

  const panels = useMemo<PanelDef[]>(
    () => [
      {
        kind: 'wiki.tree',
        title: t('wiki.tree.tab'),
        closable: true,
        popout: true,
        gmOnly: true,
        render: () => <TreePanel />,
      },
      {
        kind: 'wiki.page',
        idPrefix: PAGE_PANEL_PREFIX,
        title: t('wiki.page.tab'),
        titleFor: (params) => pagePanelTitle(tree, params.pageId, t('wiki.untitled')),
        closable: true,
        popout: true,
        gmOnly: true,
        render: (params) => (
          <PagePanel key={String(params.pageId)} pageId={String(params.pageId)} />
        ),
      },
    ],
    [t, tree],
  )

  const isPanelValid = useCallback(
    (id: string) => {
      const pageId = pageIdOfPanel(id)
      return pageId === null || tree?.some((n) => n.id === pageId) === true
    },
    [tree],
  )

  const onPanelsChange = useCallback((ids: string[], api: DockviewApi) => {
    const pages = ids.map(pageIdOfPanel).filter((id): id is string => id !== null)
    wikiStore.getState().setOpen(pages)
    // With no page open the right side is an empty group that says so; dockview drops a group with its last panel.
    const treePanel = api.getPanel('wiki.tree')
    if (
      pages.length === 0 &&
      treePanel !== undefined &&
      api.groups.every((g) => g.panels.length > 0)
    ) {
      api.addGroup({ referencePanel: treePanel, direction: 'right' })
    }
  }, [])

  const defaultLayout = useCallback(
    (api: DockviewApi) => {
      const treeDef = panels.find((p) => p.kind === 'wiki.tree')
      if (treeDef === undefined) return
      const treePanel = api.addPanel(panelOptions(treeDef))
      api.addGroup({ referencePanel: treePanel, direction: 'right' })
      const rem = parseFloat(getComputedStyle(document.documentElement).fontSize) || 16
      treePanel.api.setSize({ width: TREE_WIDTH_REM * rem })
    },
    [panels],
  )

  const emptyState = useMemo(
    () => (
      <div className="wiki-empty">
        <p>{t('wiki.page.empty')}</p>
        <Button
          variant="secondary"
          fullWidth={false}
          onClick={() => {
            void actions.create(null)
          }}
        >
          {t('wiki.page.newPage')}
        </Button>
      </div>
    ),
    [t, actions],
  )

  if (!isGm) return null

  if (campaign === 'missing') {
    return (
      <AuthLayout title={t('notFound.title')}>
        <p>
          <Link to="/">{t('notFound.home')}</Link>
        </p>
      </AuthLayout>
    )
  }

  const name = campaign === null ? '' : campaign.name
  const hasGame = campaign !== null && campaign.activeGameSession !== null
  const key = layoutKey(accountId, campaignId, 'wiki')

  const bar = (
    <header className="wikibar">
      <h1 className="wikibar__campaign">{name}</h1>
      <nav className="wikibar__nav" aria-label={t('wiki.nav')}>
        {hasGame && (
          <a
            className="wikibar__link"
            href="/table"
            onClick={(e) => {
              e.preventDefault()
              navigate('/table')
            }}
          >
            {t('wiki.topbar.table')}
          </a>
        )}
        <a className="wikibar__link" href={`/wiki/${campaignId}`} aria-current="page">
          {t('wiki.topbar.wiki')}
        </a>
      </nav>
      {!narrow && (
        <button
          type="button"
          className="wikibar__button"
          onClick={() => {
            resetLayout(key)
          }}
        >
          {t('workspace.reset')}
        </button>
      )}
      <button
        type="button"
        className="wikibar__button"
        onClick={() => {
          navigate('/')
        }}
      >
        {t('notFound.home')}
      </button>
    </header>
  )

  const status = (
    <p className="wiki__notice" role="status">
      {notice === null ? '' : t(notice)}
    </p>
  )

  let body: React.ReactNode
  if (tree === null) {
    body = treeFailed ? (
      <div className="wiki-empty">
        <ErrorNote>{t('wiki.loadFailed')}</ErrorNote>
        <Button variant="secondary" fullWidth={false} onClick={load}>
          {t('wiki.retry')}
        </Button>
      </div>
    ) : (
      <p className="wiki-empty">{t('wiki.loading')}</p>
    )
  } else if (narrow) {
    body = (
      <>
        <main className="wiki__narrow">
          {narrowTab === 'tree' || active === null ? (
            <TreePanel />
          ) : (
            <PagePanel key={active} pageId={active} />
          )}
        </main>
        <div className="wiki-tabs" role="tablist" aria-label={t('wiki.nav')}>
          {(['tree', 'page'] as const).map((tab) => (
            <button
              key={tab}
              type="button"
              role="tab"
              className="wiki-tabs__tab"
              aria-selected={narrowTab === tab}
              onClick={() => {
                setNarrowTab(tab)
              }}
            >
              {t(tab === 'tree' ? 'wiki.tree.tab' : 'wiki.page.tab')}
            </button>
          ))}
        </div>
      </>
    )
  } else {
    body = (
      <Workspace
        key={key}
        storageKey={key}
        panels={panels}
        defaultLayout={defaultLayout}
        popoutTitle={(panel) => t('workspace.popoutTitle', { panel, campaign: name })}
        isPanelValid={isPanelValid}
        onPanelsChange={onPanelsChange}
        emptyState={emptyState}
      />
    )
  }

  return (
    <WikiContext value={actions}>
      <div className={narrow ? 'wiki wiki--narrow' : 'wiki'}>
        {bar}
        {body}
        {status}
      </div>
    </WikiContext>
  )
}
