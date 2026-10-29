import type { Editor, JSONContent } from '@tiptap/core'
import { EditorContent, useEditor } from '@tiptap/react'
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { BubbleBar } from './BubbleBar'
import { createExtensions } from './extensions'
import { HandlersRef, type EditorHandlers, type MentionItem } from './handlers'
import { createMentionSuggestion } from './mentionSuggestion'
import { SlashMenu } from './SlashMenu'
import './editor.css'

export type { MentionItem }

export interface WikiEditorProps {
  /** content is the initial document (schema v1 JSON); later changes of the prop are ignored. */
  content: unknown
  editable: boolean
  /** onChange gets the document after every change (editor.getJSON()). */
  onChange?: (doc: unknown) => void
  /** mentions returns the items for the "@" menu; the next task wires it to the server. */
  mentions?: (query: string) => Promise<MentionItem[]>
  /** resolveLabel returns the current name of a mention target, or null when it is gone. */
  resolveLabel?: (kind: string, id: string) => string | null
  /** onMentionClick and onRoll are called on a click of a mention or an inline roll (read and edit mode). */
  onMentionClick?: (kind: string, id: string) => void
  onRoll?: (formula: string, visibility: 'gm' | 'public' | 'blind') => void
  /** uploadImage stores a pasted or dropped image and returns its asset id; absent — images are not accepted. */
  uploadImage?: (file: File) => Promise<string>
  /** placeholder of an empty document. */
  placeholder?: string
  /** extraActions are added to the bar over a selection (the next task adds "show to players"). */
  extraActions?: ReactNode
}

function pickHandlers(props: WikiEditorProps): EditorHandlers {
  return {
    onChange: props.onChange,
    mentions: props.mentions,
    resolveLabel: props.resolveLabel,
    onMentionClick: props.onMentionClick,
    onRoll: props.onRoll,
    uploadImage: props.uploadImage,
    placeholder: props.placeholder,
  }
}

function imageFiles(files: FileList | null | undefined): File[] {
  return files ? Array.from(files).filter((f) => f.type.startsWith('image/')) : []
}

/** insertImages uploads the files one by one and puts each in the document as a block image. */
async function insertImages(
  editor: Editor,
  handlers: HandlersRef,
  files: File[],
  at?: number,
): Promise<void> {
  const upload = handlers.current.uploadImage
  if (!upload) {
    return
  }
  let pos = at
  for (const file of files) {
    let assetId: string
    try {
      assetId = await upload(file)
    } catch {
      // The failure is shown by uploadImage itself: it knows the reason.
      continue
    }
    const size = editor.state.doc.content.size
    const target = Math.min(pos ?? editor.state.selection.to, size)
    editor
      .chain()
      .focus()
      .insertContentAt(target, { type: 'image', attrs: { assetId, alt: '' } })
      .run()
    pos = undefined
  }
}

function pickImage(editor: Editor, handlers: HandlersRef): void {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = 'image/*'
  input.addEventListener('change', () => {
    void insertImages(editor, handlers, imageFiles(input.files))
  })
  input.click()
}

/** WikiEditor is the block editor of wiki pages: it reads and writes documents of the wiki schema v1. */
export function WikiEditor(props: WikiEditorProps) {
  const [handlers] = useState(() => new HandlersRef(pickHandlers(props)))
  useLayoutEffect(() => {
    handlers.update(pickHandlers(props))
  })

  // The extensions are made once: they read the callbacks through the ref.
  const [extensions] = useState(() =>
    createExtensions({
      handlers,
      mentionSuggestion: createMentionSuggestion(handlers),
      extra: [
        SlashMenu.configure({
          canUpload: () => handlers.current.uploadImage !== undefined,
          pickImage: (editor) => {
            pickImage(editor, handlers)
          },
        }),
      ],
    }),
  )

  const editorRef = useRef<Editor | null>(null)
  const editor = useEditor({
    extensions,
    content: props.content as JSONContent,
    editable: props.editable,
    onCreate: ({ editor: e }) => {
      editorRef.current = e
    },
    onUpdate: ({ editor: e }) => {
      handlers.current.onChange?.(e.getJSON())
    },
    editorProps: {
      attributes: { class: 'wiki-editor__content' },
      handlePaste: (_view, event) => {
        const files = imageFiles(event.clipboardData?.files)
        if (files.length === 0) {
          return false
        }
        if (editorRef.current) {
          void insertImages(editorRef.current, handlers, files)
        }
        return true
      },
      handleDrop: (view, event) => {
        const files = imageFiles(event.dataTransfer?.files)
        if (files.length === 0) {
          return false
        }
        const at = view.posAtCoords({ left: event.clientX, top: event.clientY })?.pos
        if (editorRef.current) {
          void insertImages(editorRef.current, handlers, files, at)
        }
        return true
      },
    },
  })

  useEffect(() => {
    editor.setEditable(props.editable, false)
  }, [editor, props.editable])

  return (
    <div className={props.editable ? 'wiki-editor' : 'wiki-editor wiki-editor--readonly'}>
      <EditorContent editor={editor} />
      <BubbleBar editor={editor} extraActions={props.extraActions} />
    </div>
  )
}
