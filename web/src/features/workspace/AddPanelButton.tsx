import type { IDockviewHeaderActionsProps } from 'dockview-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PopupMenu, type MenuItem } from './PanelTab'
import { panelOptions, useWorkspace } from './panels'

/** AddPanelButton is the "+" at the end of a group's tabs: it opens a closed panel in that group. */
export function AddPanelButton({ containerApi, group }: IDockviewHeaderActionsProps) {
  const { t } = useTranslation()
  const { panels } = useWorkspace()
  const button = useRef<HTMLButtonElement>(null)
  const [menu, setMenu] = useState<{ x: number; y: number; doc: Document } | null>(null)
  // The list of closed panels changes when any panel is added or closed.
  const [, setVersion] = useState(0)

  useEffect(() => {
    const bump = () => {
      setVersion((v) => v + 1)
    }
    const added = containerApi.onDidAddPanel(bump)
    const removed = containerApi.onDidRemovePanel(bump)
    return () => {
      added.dispose()
      removed.dispose()
    }
  }, [containerApi])

  const open = new Set(containerApi.panels.map((p) => p.id))
  // Panels that exist many times (wiki pages) are opened from the tree.
  const closed = panels.filter((p) => p.idPrefix === undefined && !open.has(p.kind))
  if (closed.length === 0) return null

  const items: MenuItem[] = closed.map((def) => ({
    key: def.kind,
    label: def.title,
    onSelect: () => {
      containerApi.addPanel({ ...panelOptions(def), position: { referenceGroup: group } })
    },
  }))

  return (
    <div className="wadd">
      <button
        ref={button}
        type="button"
        className="wadd__button"
        aria-label={t('workspace.add')}
        aria-haspopup="menu"
        aria-expanded={menu !== null}
        onClick={() => {
          const el = button.current
          if (el === null) return
          const rect = el.getBoundingClientRect()
          setMenu((now) =>
            now === null ? { x: rect.right, y: rect.bottom + 4, doc: el.ownerDocument } : null,
          )
        }}
      >
        <span aria-hidden="true">+</span>
      </button>
      {menu !== null && (
        <PopupMenu
          doc={menu.doc}
          at={{ x: menu.x, y: menu.y, align: 'end' }}
          label={t('workspace.add')}
          items={items}
          onClose={(restoreFocus) => {
            setMenu(null)
            if (restoreFocus) button.current?.focus()
          }}
        />
      )}
    </div>
  )
}
