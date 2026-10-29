import { createContext, useContext } from 'react'

export type PanelKind = 'scene' | 'chat' | 'combat' | 'scenes'

/** PanelDef is a row of the panel catalogue (docs/architecture/workspace.md). */
export interface PanelDef {
  kind: PanelKind
  /** title is the translated name shown on the tab. */
  title: string
  closable: boolean
  /** popout tells whether the panel may live in a window of its own. */
  popout: boolean
  gmOnly: boolean
  render: () => React.ReactNode
}

/** MIN_PANEL is the smallest a panel gets; the canvas needs more room. */
export const MIN_PANEL = { width: 240, height: 160 }
export const MIN_SCENE = { width: 320, height: 240 }

/** panelOptions are the options a single panel is added to dockview with: id and component are its kind. */
export function panelOptions(def: PanelDef) {
  const min = def.kind === 'scene' ? MIN_SCENE : MIN_PANEL
  return {
    id: def.kind,
    component: def.kind,
    title: def.title,
    minimumWidth: min.width,
    minimumHeight: min.height,
  }
}

/** findPanel returns the catalogue row of a kind, if the role has it. */
export function findPanel(panels: PanelDef[], kind: string): PanelDef | undefined {
  return panels.find((p) => p.kind === kind)
}

export interface WorkspaceValue {
  panels: PanelDef[]
  /** popout moves the panel of the kind into a window of its own. */
  popout: (kind: string) => void
}

export const WorkspaceContext = createContext<WorkspaceValue | null>(null)

/** useWorkspace gives the tabs and the panels what the Workspace around them knows. */
export function useWorkspace(): WorkspaceValue {
  const value = useContext(WorkspaceContext)
  if (value === null) throw new Error('useWorkspace: no Workspace around')
  return value
}
