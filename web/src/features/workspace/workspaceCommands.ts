import { useCallback } from 'react'
import type { PanelKind, PanelParams } from './panels'

export const RESET_EVENT = 'dndvtt:workspace-reset'
export const OPEN_EVENT = 'dndvtt:workspace-open'
export const OPEN_INSTANCE_EVENT = 'dndvtt:workspace-open-instance'

/** InstanceRequest asks for one of the panels that exist many times (a wiki page). */
export interface InstanceRequest {
  kind: PanelKind
  key: string
  params: PanelParams
  /** newGroup puts the panel in a group of its own, to the right of the active one. */
  newGroup: boolean
}

/** useWorkspaceOpenInstance gives the function that shows such a panel: the open one is activated, otherwise it is added. */
export function useWorkspaceOpenInstance(): (request: InstanceRequest) => void {
  return useCallback((request: InstanceRequest) => {
    window.dispatchEvent(new CustomEvent<InstanceRequest>(OPEN_INSTANCE_EVENT, { detail: request }))
  }, [])
}

/** useWorkspaceReset gives the function that forgets the saved layout of a key and puts the default one back. */
export function useWorkspaceReset(): (storageKey: string) => void {
  return useCallback((storageKey: string) => {
    window.dispatchEvent(new CustomEvent<string>(RESET_EVENT, { detail: storageKey }))
  }, [])
}

/** useWorkspaceOpen gives the function that shows a panel: it activates the open one and opens a closed one next to the other panels. */
export function useWorkspaceOpen(): (kind: PanelKind) => void {
  return useCallback((kind: PanelKind) => {
    window.dispatchEvent(new CustomEvent<PanelKind>(OPEN_EVENT, { detail: kind }))
  }, [])
}
