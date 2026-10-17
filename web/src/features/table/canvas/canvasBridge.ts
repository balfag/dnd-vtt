import { createStore, type StoreApi } from 'zustand/vanilla'

/** FocusRequest asks the canvas to select a token and bring it to the middle of the view. */
export interface FocusRequest {
  tokenId: string
  /** Grows with every request, so asking for the same token again is a new request. */
  n: number
}

export interface CanvasBridgeState {
  /** The token selected on the canvas; the canvas keeps it, the panels read it. */
  selectedTokenId: string | null
  focus: FocusRequest | null
  setSelected(id: string | null): void
  focusToken(id: string): void
}

/**
 * createCanvasBridge makes the small channel between the canvas, which owns the selection and the
 * camera, and the side panels, which show the selection and ask to look at a token.
 */
export function createCanvasBridge(): StoreApi<CanvasBridgeState> {
  return createStore<CanvasBridgeState>()((set) => ({
    selectedTokenId: null,
    focus: null,
    setSelected: (selectedTokenId) => {
      set({ selectedTokenId })
    },
    focusToken: (tokenId) => {
      set((s) => ({ focus: { tokenId, n: (s.focus?.n ?? 0) + 1 } }))
    },
  }))
}

export const canvasBridge: StoreApi<CanvasBridgeState> = createCanvasBridge()
