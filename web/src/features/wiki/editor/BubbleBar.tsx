import type { Editor } from '@tiptap/core'
import { useEditorState } from '@tiptap/react'
import { BubbleMenu } from '@tiptap/react/menus'
import { useRef, useState, type ReactNode, type SyntheticEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { normalizeHref } from './rules'

interface BubbleBarProps {
  editor: Editor
  /** extraActions are added after the standard buttons (the "show to players" button of handouts). */
  extraActions?: ReactNode
}

/** BubbleBar is the floating bar over a text selection: bold, italic, link. */
export function BubbleBar({ editor, extraActions }: BubbleBarProps) {
  const { t } = useTranslation()
  const barRef = useRef<HTMLDivElement | null>(null)
  const inputRef = useRef<HTMLInputElement | null>(null)
  const [linking, setLinking] = useState(false)
  const [invalid, setInvalid] = useState(false)

  const state = useEditorState({
    editor,
    selector: ({ editor: e }) => ({
      bold: e.isActive('bold'),
      italic: e.isActive('italic'),
      link: e.isActive('link'),
      href: (e.getAttributes('link').href as string | undefined) ?? '',
    }),
  })

  const closeLink = () => {
    setLinking(false)
    setInvalid(false)
    editor.commands.focus()
  }

  const submit = (e: SyntheticEvent) => {
    e.preventDefault()
    const value = inputRef.current?.value ?? ''
    if (value.trim() === '') {
      editor.chain().focus().extendMarkRange('link').unsetLink().run()
      closeLink()
      return
    }
    const href = normalizeHref(value)
    if (href === null) {
      setInvalid(true)
      return
    }
    editor.chain().focus().extendMarkRange('link').setLink({ href }).run()
    closeLink()
  }

  // The buttons must not take the focus from the editor: the selection would be lost.
  const keepFocus = (e: React.MouseEvent) => {
    e.preventDefault()
  }

  return (
    <BubbleMenu
      editor={editor}
      options={{ placement: 'top', offset: 8 }}
      shouldShow={({ view, state: s }) =>
        editor.isEditable &&
        !s.selection.empty &&
        !editor.isActive('codeBlock') &&
        (view.hasFocus() || (barRef.current?.contains(document.activeElement) ?? false))
      }
    >
      <div
        className="wiki-bubble"
        ref={barRef}
        role="toolbar"
        aria-label={t('wiki.editor.bubble.label')}
      >
        {linking ? (
          <form className="wiki-bubble__link" onSubmit={submit}>
            <input
              ref={(el) => {
                inputRef.current = el
                el?.focus()
              }}
              type="text"
              className="wiki-bubble__input"
              defaultValue={state.href}
              placeholder={t('wiki.editor.bubble.linkPlaceholder')}
              aria-label={t('wiki.editor.bubble.link')}
              aria-invalid={invalid}
              onChange={() => {
                setInvalid(false)
              }}
              onKeyDown={(e) => {
                if (e.key === 'Escape') {
                  e.preventDefault()
                  closeLink()
                }
              }}
            />
            {invalid && (
              <span className="wiki-bubble__error" role="alert">
                {t('wiki.editor.bubble.linkInvalid')}
              </span>
            )}
          </form>
        ) : (
          <>
            <button
              type="button"
              className="wiki-bubble__button wiki-bubble__button--bold"
              aria-pressed={state.bold}
              title={t('wiki.editor.bubble.boldTitle')}
              onMouseDown={keepFocus}
              onClick={() => editor.chain().focus().toggleBold().run()}
            >
              {t('wiki.editor.bubble.bold')}
            </button>
            <button
              type="button"
              className="wiki-bubble__button wiki-bubble__button--italic"
              aria-pressed={state.italic}
              title={t('wiki.editor.bubble.italicTitle')}
              onMouseDown={keepFocus}
              onClick={() => editor.chain().focus().toggleItalic().run()}
            >
              {t('wiki.editor.bubble.italic')}
            </button>
            <button
              type="button"
              className="wiki-bubble__button"
              aria-pressed={state.link}
              onMouseDown={keepFocus}
              onClick={() => {
                setLinking(true)
              }}
            >
              {t('wiki.editor.bubble.link')}
            </button>
            {extraActions}
          </>
        )}
      </div>
    </BubbleMenu>
  )
}
