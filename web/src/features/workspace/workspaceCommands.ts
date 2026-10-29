import { useCallback } from 'react'
import type { PanelKind } from './panels'

export const RESET_EVENT = 'dndvtt:workspace-reset'
export const OPEN_EVENT = 'dndvtt:workspace-open'

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
