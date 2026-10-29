/** MentionItem is one entry of the "@" menu. */
export interface MentionItem {
  kind: 'page' | 'scene' | 'token'
  id: string
  label: string
  detail: string
}

/**
 * EditorHandlers are the callbacks of WikiEditor. The editor and its node views are created once,
 * so they read the current callbacks from a ref at the moment of use.
 */
export interface EditorHandlers {
  onChange?: (doc: unknown) => void
  mentions?: (query: string) => Promise<MentionItem[]>
  resolveLabel?: (kind: string, id: string) => string | null | undefined
  onMentionClick?: (kind: string, id: string) => void
  onRoll?: (formula: string, visibility: 'gm' | 'public' | 'blind') => void
  uploadImage?: (file: File) => Promise<string>
  placeholder?: string
}

/** HandlersRef holds the current callbacks; the editor keeps one for its whole life. */
export class HandlersRef {
  current: EditorHandlers

  constructor(initial: EditorHandlers = {}) {
    this.current = initial
  }

  update(next: EditorHandlers): void {
    this.current = next
  }
}
