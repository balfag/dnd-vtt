import { createContext, useContext } from 'react'

export type PanelKind = 'scene' | 'chat' | 'combat' | 'scenes' | 'wiki.tree' | 'wiki.page'

/** PanelDef is a row of the panel catalogue (docs/architecture/workspace.md). */
export interface PanelDef {
  kind: PanelKind
  /** title is the translated name shown on the tab. */
  title: string
  closable: boolean
  /** popout tells whether the panel may live in a window of its own. */
  popout: boolean
  gmOnly: boolean
  /** idPrefix marks a panel that may exist many times: its id is the prefix and a key (page:<pageId>). */
  idPrefix?: string
  /** titleFor is the title of one such panel, from its parameters; absent: title. */
  titleFor?: (params: PanelParams) => string
  render: (params: PanelParams) => React.ReactNode
}

/** PanelParams are the parameters a panel was added with (pageId of a wiki page). */
export type PanelParams = Record<string, unknown>

/** MIN_PANEL is the smallest a panel gets; the canvas needs more room. */
export const MIN_PANEL = { width: 240, height: 160 }
export const MIN_SCENE = { width: 320, height: 240 }

/** paramsOf narrows the parameters dockview hands over (typed `any`) to the object a panel was added with. */
export function paramsOf(params: unknown): PanelParams {
  return typeof params === 'object' && params !== null ? (params as PanelParams) : {}
}

/** panelId is the dockview id of a panel: its kind, or the prefix and key of a panel that exists many times. */
export function panelId(def: PanelDef, key?: string): string {
  return def.idPrefix === undefined ? def.kind : `${def.idPrefix}${key ?? ''}`
}

/** panelOptions are the options a panel is added to dockview with; the component is its kind, the id is told by panelId. */
export function panelOptions(def: PanelDef, instance?: { key: string; params: PanelParams }) {
  const min = def.kind === 'scene' ? MIN_SCENE : MIN_PANEL
  return {
    id: panelId(def, instance?.key),
    component: def.kind,
    title: def.titleFor !== undefined && instance ? def.titleFor(instance.params) : def.title,
    ...(instance ? { params: instance.params } : {}),
    minimumWidth: min.width,
    minimumHeight: min.height,
  }
}

/** findPanel returns the catalogue row of a panel id, if the role has it. */
export function findPanel(panels: PanelDef[], id: string): PanelDef | undefined {
  return panels.find(
    (p) => p.kind === id || (p.idPrefix !== undefined && id.startsWith(p.idPrefix)),
  )
}

export interface WorkspaceValue {
  panels: PanelDef[]
  /** popout moves the panel with the id into a window of its own. */
  popout: (id: string) => void
  /** emptyState is what a group with no panels shows. */
  emptyState?: React.ReactNode
}

export const WorkspaceContext = createContext<WorkspaceValue | null>(null)

/** useWorkspace gives the tabs and the panels what the Workspace around them knows. */
export function useWorkspace(): WorkspaceValue {
  const value = useContext(WorkspaceContext)
  if (value === null) throw new Error('useWorkspace: no Workspace around')
  return value
}
