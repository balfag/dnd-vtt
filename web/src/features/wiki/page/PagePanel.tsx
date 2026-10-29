import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../../../shared/api/client'
import { uploadAsset } from '../../../shared/api/assets'
import {
  getWikiPage,
  saveWikiPage,
  searchMentions,
  setPageTags,
  type RefInfo,
  type Tag,
  type WikiPage,
} from '../../../shared/api/wiki'
import { Button } from '../../../ui/Button'
import { ErrorNote } from '../../../ui/ErrorNote'
import { isSearchShortcut } from '../search/query'
import { wikiStore } from '../store'
import { pageEditors, useWikiActions } from '../wikiContext'
import { Backlinks } from './Backlinks'
import { createAutosaver, type Autosaver, type Draft } from './autosave'
import { debounceSearch } from './mentions'
import { PageHeader } from './PageHeader'

// The editor (TipTap and ProseMirror) is a chunk of its own.
const WikiEditor = lazy(() =>
  import('../editor/WikiEditor').then((m) => ({ default: m.WikiEditor })),
)

const MENTION_DEBOUNCE_MS = 150
/** KEEPALIVE_LIMIT is the biggest body a request may have to outlive the page (the browsers allow 64 KiB). */
const KEEPALIVE_LIMIT = 60 * 1024

type Load =
  | { status: 'loading' }
  | { status: 'failed'; gone: boolean }
  | { status: 'ready'; page: WikiPage; epoch: number }

/** PagePanel is the panel "Page": it reads the page and shows it for editing. */
export function PagePanel({ pageId }: { pageId: string }) {
  const { t } = useTranslation()
  const [load, setLoad] = useState<Load>({ status: 'loading' })
  const epoch = useRef(0)

  // fetchPage reads the page; the result is ignored once cancelled() is true.
  const fetchPage = useCallback(
    (cancelled: () => boolean) => {
      getWikiPage(pageId).then(
        (page) => {
          if (!cancelled()) setLoad({ status: 'ready', page, epoch: ++epoch.current })
        },
        (err: unknown) => {
          if (!cancelled())
            setLoad({ status: 'failed', gone: err instanceof ApiError && err.status === 404 })
        },
      )
    },
    [pageId],
  )

  // read is the "load again" button: the edits made here are lost.
  const read = useCallback(() => {
    setLoad({ status: 'loading' })
    fetchPage(() => false)
  }, [fetchPage])

  useEffect(() => {
    let stop = false
    fetchPage(() => stop)
    return () => {
      stop = true
    }
  }, [fetchPage])

  if (load.status === 'loading') {
    return <p className="page__note">{t('wiki.loading')}</p>
  }
  if (load.status === 'failed') {
    return (
      <div className="page__note">
        <ErrorNote>{t(load.gone ? 'wiki.page.gone' : 'wiki.page.loadFailed')}</ErrorNote>
        {!load.gone && (
          <Button variant="secondary" fullWidth={false} onClick={read}>
            {t('wiki.retry')}
          </Button>
        )}
      </div>
    )
  }
  return <PageView key={load.epoch} page={load.page} onReload={read} />
}

interface PageViewProps {
  page: WikiPage
  /** onReload reads the page from the server again; the edits made here are lost. */
  onReload: () => void
}

/** PageView edits one loaded page; it has one autosaver for its whole life. */
function PageView({ page, onReload }: PageViewProps) {
  const { t } = useTranslation()
  const actions = useWikiActions()
  const [title, setTitle] = useState(page.title)
  const [icon, setIcon] = useState(page.icon)
  const [cover, setCover] = useState(page.coverAssetId)
  const [refs, setRefs] = useState<Record<string, RefInfo>>(page.refs)
  const [savedAt, setSavedAt] = useState(page.updatedAt)
  const [tags, setTags] = useState<Tag[]>(page.tags)
  const draft = useRef<Draft>({
    title: page.title,
    icon: page.icon,
    coverAssetId: page.coverAssetId,
    content: page.content,
  })

  const [saver] = useState<Autosaver>(() =>
    createAutosaver(page.id, page.version, saveWikiPage, (saved) => {
      setRefs(saved.refs)
      setSavedAt(saved.updatedAt)
    }),
  )
  const saveState = useSyncExternalStore(
    (fn) => saver.subscribe(fn),
    () => saver.state,
  )

  const edit = useCallback(
    (patch: Partial<Draft>) => {
      draft.current = { ...draft.current, ...patch }
      saver.change(draft.current)
    },
    [saver],
  )

  const changeTitle = useCallback(
    (next: string) => {
      setTitle(next)
      edit({ title: next })
      wikiStore.getState().retitle(page.id, next)
    },
    [edit, page.id],
  )

  // The tree renames a page that is open through its panel: a second writer would make a conflict.
  useEffect(() => {
    pageEditors.set(page.id, { setTitle: changeTitle })
    return () => {
      if (pageEditors.get(page.id)?.setTitle === changeTitle) pageEditors.delete(page.id)
    }
  }, [page.id, changeTitle])

  // Whatever is left unsaved goes out when the panel closes or the tab is hidden.
  useEffect(() => {
    const onHide = () => {
      const size = new TextEncoder().encode(JSON.stringify(draft.current)).length
      void saver.flush({ keepalive: size <= KEEPALIVE_LIMIT })
    }
    window.addEventListener('pagehide', onHide)
    return () => {
      window.removeEventListener('pagehide', onHide)
      void saver.flush()
    }
  }, [saver])

  // Tags are saved at once and apart from the text, without a version.
  const changeTags = useCallback(
    (names: string[]) => {
      const before = tags
      setTags(names.map((name) => tags.find((x) => x.name === name) ?? { id: `new:${name}`, name }))
      setPageTags(page.id, names).then(
        (saved) => {
          setTags(saved)
          wikiStore.getState().bumpTools()
        },
        () => {
          setTags(before)
          actions.notify('wiki.tools.tags.pageFailed')
        },
      )
    },
    [actions, page.id, tags],
  )

  const mentions = useMemo(
    () => debounceSearch((q: string) => searchMentions(page.campaignId, q), MENTION_DEBOUNCE_MS),
    [page.campaignId],
  )

  const resolveLabel = useCallback(
    (kind: string, id: string): string | null | undefined => {
      const ref = refs[`${kind}:${id}`]
      if (ref === undefined) return undefined
      return ref.exists ? ref.label : null
    },
    [refs],
  )

  const uploadImage = useCallback(
    async (file: File): Promise<string> => {
      try {
        return (await uploadAsset(page.campaignId, file, 'other')).id
      } catch (err) {
        actions.notify('wiki.page.imageFailed')
        throw err
      }
    },
    [actions, page.campaignId],
  )

  const onMentionClick = useCallback(
    (kind: string, id: string) => {
      if (kind === 'page') actions.open(id)
      else actions.notify('wiki.mention.tableOnly')
    },
    [actions],
  )

  return (
    <article
      className="page"
      onKeyDown={(e) => {
        if (isSearchShortcut(e.nativeEvent)) {
          e.preventDefault()
          actions.focusSearch()
        }
      }}
    >
      {saveState === 'conflict' && (
        <div className="page__conflict" role="alert">
          <p>{t('wiki.page.conflict.text')}</p>
          <Button variant="secondary" fullWidth={false} onClick={onReload}>
            {t('wiki.page.conflict.reload')}
          </Button>
          <Button
            variant="secondary"
            fullWidth={false}
            onClick={() => {
              void saver.overwrite()
            }}
          >
            {t('wiki.page.conflict.keep')}
          </Button>
        </div>
      )}
      <div className="page__scroll">
        <div className="page__column">
          <PageHeader
            campaignId={page.campaignId}
            pageId={page.id}
            tags={tags}
            onTags={changeTags}
            path={page.path}
            title={title}
            icon={icon}
            coverAssetId={cover}
            saveState={saveState}
            saveError={saver.error}
            onTitle={changeTitle}
            onIcon={(next) => {
              setIcon(next)
              edit({ icon: next })
              wikiStore.getState().retitle(page.id, title, next)
            }}
            onCover={(assetId) => {
              setCover(assetId)
              edit({ coverAssetId: assetId })
            }}
            onOpenPath={(id) => {
              actions.open(id)
            }}
            onRetry={() => {
              void saver.flush()
            }}
          />
          <Suspense fallback={null}>
            <WikiEditor
              content={page.content}
              editable
              onChange={(doc) => {
                edit({ content: doc })
              }}
              mentions={mentions}
              resolveLabel={resolveLabel}
              onMentionClick={onMentionClick}
              uploadImage={uploadImage}
            />
          </Suspense>
          <Backlinks pageId={page.id} savedAt={savedAt} />
        </div>
      </div>
    </article>
  )
}
