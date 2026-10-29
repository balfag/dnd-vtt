import {
  DockviewReact,
  type DockviewApi,
  type DockviewReadyEvent,
  type DockviewTheme,
  type IDockviewPanelProps,
} from 'dockview-react'
import 'dockview-react/dist/styles/dockview.css'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AddPanelButton } from './AddPanelButton'
import { clearLayout, loadLayout, sanitizeLayout, saveLayout } from './layout'
import { PanelTab } from './PanelTab'
import { OPEN_EVENT, RESET_EVENT } from './workspaceCommands'
import {
  findPanel,
  panelOptions,
  WorkspaceContext,
  useWorkspace,
  type PanelDef,
  type PanelKind,
  type WorkspaceValue,
} from './panels'
import './workspace.css'

export interface WorkspaceProps {
  storageKey: string
  /** panels are the panels this role has. */
  panels: PanelDef[]
  /** defaultLayout fills an empty dockview with the layout of a first visit. */
  defaultLayout: (api: DockviewApi) => void
  /** popoutTitle is the title of the window of a detached panel. */
  popoutTitle: (panelTitle: string) => string
}

const SAVE_DELAY_MS = 500
const NOTICE_MS = 6000
const POPOUT_URL = '/popout.html'

// The overlay is mounted at the root of the workspace (not in the group), so that the fog around the drop target covers all of it.
const theme: DockviewTheme = {
  name: 'dndvtt',
  className: 'dockview-theme-dndvtt',
  colorScheme: 'dark',
  gap: 0,
  dndOverlayMounting: 'absolute',
  dndPanelOverlay: 'group',
}

const kinds: PanelKind[] = ['scene', 'chat', 'combat', 'scenes']

function PanelContent({ api }: IDockviewPanelProps) {
  const { panels } = useWorkspace()
  return <>{findPanel(panels, api.id)?.render()}</>
}

// One component per kind of the catalogue; a panel's id is its kind.
const components = Object.fromEntries(kinds.map((k) => [k, PanelContent]))

/** Workspace is the area of tiled panels: tabs, splits, resizing and windows of their own. */
export function Workspace({ storageKey, panels, defaultLayout, popoutTitle }: WorkspaceProps) {
  const { t } = useTranslation()
  const [api, setApi] = useState<DockviewApi | null>(null)
  const [blocked, setBlocked] = useState(false)
  // The callbacks of dockview are made once; they read the latest props from here.
  const latest = useRef({ storageKey, panels, defaultLayout, popoutTitle })
  useEffect(() => {
    latest.current = { storageKey, panels, defaultLayout, popoutTitle }
  })

  const buildDefault = useCallback((dock: DockviewApi) => {
    dock.clear()
    latest.current.defaultLayout(dock)
  }, [])

  const onReady = useCallback(
    ({ api: dock }: DockviewReadyEvent) => {
      const { storageKey: key, panels: defs } = latest.current
      const saved = loadLayout(key)
      const layout =
        saved === null ? null : sanitizeLayout(saved, (k) => findPanel(defs, k) !== undefined)
      let restored = false
      if (layout !== null) {
        try {
          dock.fromJSON(layout)
          // A layout without a panel that cannot be closed (the canvas) is no use.
          restored = defs.every((d) => d.closable || dock.getPanel(d.kind) !== undefined)
        } catch {
          restored = false
        }
      }
      if (!restored) buildDefault(dock)
      setApi(dock)
    },
    [buildDefault],
  )

  // Save the layout 500 ms after the last change.
  useEffect(() => {
    if (api === null) return
    let timer: ReturnType<typeof setTimeout> | undefined
    const sub = api.onDidLayoutChange(() => {
      clearTimeout(timer)
      timer = setTimeout(() => {
        try {
          saveLayout(latest.current.storageKey, api.toJSON())
        } catch {
          // The dockview may be gone already.
        }
      }, SAVE_DELAY_MS)
    })
    return () => {
      clearTimeout(timer)
      sub.dispose()
    }
  }, [api])

  // "Reset layout" in the top bar.
  useEffect(() => {
    if (api === null) return
    const onReset = (e: Event) => {
      if (!(e instanceof CustomEvent) || e.detail !== latest.current.storageKey) return
      clearLayout(latest.current.storageKey)
      buildDefault(api)
    }
    window.addEventListener(RESET_EVENT, onReset)
    return () => {
      window.removeEventListener(RESET_EVENT, onReset)
    }
  }, [api, buildDefault])

  // The titles are saved with the layout, in the language of that day.
  const titles = panels.map((p) => `${p.kind}:${p.title}`).join('|')
  useEffect(() => {
    if (api === null) return
    for (const panel of api.panels) {
      const def = findPanel(latest.current.panels, panel.id)
      if (def !== undefined && panel.title !== def.title) panel.api.setTitle(def.title)
    }
  }, [api, titles])

  // "Scenes" in the top bar.
  useEffect(() => {
    if (api === null) return
    const onOpen = (e: Event) => {
      if (!(e instanceof CustomEvent) || typeof e.detail !== 'string') return
      const kind = e.detail
      const open = api.getPanel(kind)
      if (open !== undefined) {
        open.api.setActive()
        return
      }
      const def = findPanel(latest.current.panels, kind)
      if (def === undefined) return
      const neighbour = api.panels.find((p) => p.id !== 'scene')
      api.addPanel({
        ...panelOptions(def),
        position:
          neighbour === undefined
            ? { referencePanel: 'scene', direction: 'right' }
            : { referencePanel: neighbour },
      })
    }
    window.addEventListener(OPEN_EVENT, onOpen)
    return () => {
      window.removeEventListener(OPEN_EVENT, onOpen)
    }
  }, [api])

  // A window the browser did not let open.
  useEffect(() => {
    if (api === null) return
    const sub = api.onDidOpenPopoutWindowFail(() => {
      setBlocked(true)
    })
    return () => {
      sub.dispose()
    }
  }, [api])
  useEffect(() => {
    if (!blocked) return
    const timer = setTimeout(() => {
      setBlocked(false)
    }, NOTICE_MS)
    return () => {
      clearTimeout(timer)
    }
  }, [blocked])

  const popout = useCallback(
    (kind: string) => {
      const panel = api?.getPanel(kind)
      const def = findPanel(latest.current.panels, kind)
      if (api === null || panel === undefined || def === undefined || !def.popout) return
      const opened: { popup?: Window } = {}
      api
        .addPopoutGroup(panel, {
          popoutUrl: POPOUT_URL,
          onDidOpen: ({ window: popup }) => {
            opened.popup = popup
          },
        })
        .then(
          (ok) => {
            if (!ok) {
              setBlocked(true)
              return
            }
            // dockview copies the title of the main page on load; the promise settles after that.
            const win = opened.popup
            const group = panel.group
            if (win === undefined) return
            const setTitle = (kind: string) => {
              const shown = findPanel(latest.current.panels, kind)
              if (shown !== undefined) win.document.title = latest.current.popoutTitle(shown.title)
            }
            setTitle(kind)
            group.api.onDidActivePanelChange((e) => {
              setTitle(e.panel.id)
            })
          },
          () => {
            setBlocked(true)
          },
        )
    },
    [api],
  )

  const value = useMemo<WorkspaceValue>(() => ({ panels, popout }), [panels, popout])
  return (
    <WorkspaceContext value={value}>
      <div className="workspace">
        <DockviewReact
          className="workspace__dock"
          components={components}
          defaultTabComponent={PanelTab}
          rightHeaderActionsComponent={AddPanelButton}
          onReady={onReady}
          theme={theme}
          disableFloatingGroups
        />
        {blocked && (
          <p className="workspace__notice" role="status">
            {t('workspace.popupBlocked')}
            <button
              type="button"
              className="workspace__notice-close"
              aria-label={t('workspace.close')}
              onClick={() => {
                setBlocked(false)
              }}
            >
              <span aria-hidden="true">✕</span>
            </button>
          </p>
        )}
      </div>
    </WorkspaceContext>
  )
}
