import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { PageNode } from '../../../shared/api/wiki'
import { Button } from '../../../ui/Button'
import { Dialog } from '../../../ui/Dialog'
import { useWiki } from '../store'
import { useWikiActions } from '../wikiContext'
import { MoveDialog } from './MoveDialog'
import { TreeRow } from './TreeRow'
import {
  buildTree,
  childrenOf,
  descendantIds,
  dropFor,
  visibleRows,
  type Drop,
  type DropZone,
} from './tree'

interface DragState {
  id: string
  /** over and zone are the row under the pointer and the part of it, when a drop there is allowed. */
  over: string | null
  zone: DropZone | null
}

/** zoneOf tells which part of a row the pointer is over: the top fourth, the bottom fourth, or the middle. */
function zoneOf(e: React.DragEvent, row: HTMLElement): DropZone {
  const rect = row.getBoundingClientRect()
  const ratio = (e.clientY - rect.top) / Math.max(rect.height, 1)
  if (ratio < 0.25) return 'before'
  if (ratio > 0.75) return 'after'
  return 'on'
}

/** TreePanel is the panel "Wiki": the page tree with its rows, menus, renaming and dragging. */
export function TreePanel() {
  const { t } = useTranslation()
  const actions = useWikiActions()
  const tree = useWiki((s) => s.tree)
  const active = useWiki((s) => s.active)
  const nodes = useMemo<PageNode[]>(() => tree ?? [], [tree])
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(new Set())
  const [focusId, setFocusId] = useState<string | null>(null)
  const [renaming, setRenaming] = useState<string | null>(null)
  const [moving, setMoving] = useState<{ page: PageNode; key: number; open: boolean } | null>(null)
  const [deleting, setDeleting] = useState<{ page: PageNode; open: boolean } | null>(null)
  const [drag, setDrag] = useState<DragState | null>(null)
  const rows = useRef(new Map<string, HTMLDivElement>())

  const items = useMemo(() => buildTree(nodes), [nodes])
  const visible = useMemo(() => visibleRows(items, expanded), [items, expanded])
  const tabStop = visible.some((r) => r.node.id === focusId)
    ? focusId
    : (visible[0]?.node.id ?? null)

  function expand(id: string) {
    setExpanded((now) => (now.has(id) ? now : new Set(now).add(id)))
  }

  function toggle(id: string) {
    setExpanded((now) => {
      const next = new Set(now)
      if (!next.delete(id)) next.add(id)
      return next
    })
  }

  function focusRow(id: string | undefined) {
    if (id === undefined) return
    setFocusId(id)
    rows.current.get(id)?.focus()
  }

  function openRow(node: PageNode, newGroup: boolean) {
    if (node.kind === 'database') {
      actions.notify('wiki.tree.databaseLater')
      return
    }
    actions.open(node.id, { newGroup })
  }

  function addChild(node: PageNode) {
    expand(node.id)
    void actions.create(node.id)
  }

  function onKeyDown(e: React.KeyboardEvent) {
    const target = e.target
    if (!(target instanceof HTMLElement) || target.getAttribute('role') !== 'treeitem') return
    const at = visible.findIndex((r) => rows.current.get(r.node.id) === target)
    const row = visible[at]
    if (row === undefined) return
    const { node } = row
    switch (e.key) {
      case 'ArrowDown':
        focusRow(visible[at + 1]?.node.id)
        break
      case 'ArrowUp':
        focusRow(visible[at - 1]?.node.id)
        break
      case 'Home':
        focusRow(visible[0]?.node.id)
        break
      case 'End':
        focusRow(visible[visible.length - 1]?.node.id)
        break
      case 'ArrowRight':
        if (row.hasChildren && !expanded.has(node.id)) expand(node.id)
        else if (row.hasChildren) focusRow(visible[at + 1]?.node.id)
        break
      case 'ArrowLeft':
        if (row.hasChildren && expanded.has(node.id)) {
          toggle(node.id)
        } else if (node.parentId !== null) {
          focusRow(node.parentId)
        }
        break
      case 'Enter':
        openRow(node, e.ctrlKey || e.metaKey)
        break
      case 'F2':
        setRenaming(node.id)
        break
      default:
        return
    }
    e.preventDefault()
  }

  function dropAt(overId: string, zone: DropZone, dragId: string): Drop | null {
    return dropFor(nodes, dragId, overId, zone)
  }

  function dropAtRootEnd(dragId: string): Drop | null {
    const last = childrenOf(nodes, null)
      .filter((n) => n.id !== dragId)
      .pop()
    return last === undefined ? null : dropFor(nodes, dragId, last.id, 'after')
  }

  function finishDrop(e: React.DragEvent, target: Drop | null, expandId?: string) {
    e.preventDefault()
    const dragged = drag
    setDrag(null)
    if (dragged === null || target === null) return
    if (expandId !== undefined) expand(expandId)
    void actions.move(dragged.id, target.parentId, target.index)
  }

  function renderRows() {
    // A level of a row among its siblings, for screen readers.
    const siblings = (node: PageNode) => {
      const list = childrenOf(
        nodes,
        node.parentId !== null && nodes.some((n) => n.id === node.parentId) ? node.parentId : null,
      )
      return { size: list.length, index: list.findIndex((n) => n.id === node.id) }
    }
    return visible.map((row) => {
      const { node } = row
      return (
        <TreeRow
          key={node.id}
          registerRow={(el) => {
            if (el) rows.current.set(node.id, el)
            else rows.current.delete(node.id)
          }}
          node={node}
          depth={row.depth}
          hasChildren={row.hasChildren}
          expanded={expanded.has(node.id)}
          selected={active === node.id}
          tabbable={tabStop === node.id}
          siblings={siblings(node)}
          renaming={renaming === node.id}
          mark={drag?.over === node.id ? drag.zone : null}
          dragging={drag?.id === node.id}
          onToggle={() => {
            toggle(node.id)
          }}
          onOpen={(newGroup) => {
            openRow(node, newGroup)
          }}
          onAddChild={() => {
            addChild(node)
          }}
          onStartRename={() => {
            setRenaming(node.id)
          }}
          onRename={(title) => {
            setRenaming(null)
            focusRow(node.id)
            void actions.rename(node.id, title)
          }}
          onCancelRename={() => {
            setRenaming(null)
            focusRow(node.id)
          }}
          onMove={() => {
            setMoving((m) => ({ page: node, key: (m?.key ?? 0) + 1, open: true }))
          }}
          onDelete={() => {
            setDeleting({ page: node, open: true })
          }}
          onFocusRow={() => {
            setFocusId(node.id)
          }}
          onDragStart={(e) => {
            e.dataTransfer.effectAllowed = 'move'
            e.dataTransfer.setData('text/plain', node.id)
            setDrag({ id: node.id, over: null, zone: null })
          }}
          onDragOver={(e) => {
            if (drag === null) return
            const zone = zoneOf(e, e.currentTarget as HTMLElement)
            const target = dropAt(node.id, zone, drag.id)
            if (target === null) {
              if (drag.over !== null) setDrag({ ...drag, over: null, zone: null })
              return
            }
            e.preventDefault()
            e.stopPropagation()
            e.dataTransfer.dropEffect = 'move'
            if (drag.over !== node.id || drag.zone !== zone)
              setDrag({ ...drag, over: node.id, zone })
          }}
          onDrop={(e) => {
            if (drag === null) return
            const zone = zoneOf(e, e.currentTarget as HTMLElement)
            e.stopPropagation()
            finishDrop(e, dropAt(node.id, zone, drag.id), zone === 'on' ? node.id : undefined)
          }}
          onDragEnd={() => {
            setDrag(null)
          }}
        />
      )
    })
  }

  const movingNode = moving?.page ?? null
  const deleteCount = deleting === null ? 0 : descendantIds(nodes, deleting.page.id).length

  return (
    <div className="tree">
      <div className="tree__head">
        <h2 className="tree__title">{t('wiki.tree.tab')}</h2>
        <button
          type="button"
          className="tree__add"
          aria-label={t('wiki.tree.add')}
          title={t('wiki.tree.add')}
          onClick={() => {
            void actions.create(null)
          }}
        >
          <span aria-hidden="true">+</span>
        </button>
      </div>
      <div
        className="tree__rows"
        role="tree"
        aria-label={t('wiki.tree.label')}
        onKeyDown={onKeyDown}
        onDragOver={(e) => {
          // The free space under the rows: the page goes to the end of the root.
          if (drag === null || (e.target as HTMLElement).closest('[role="treeitem"]') !== null)
            return
          if (dropAtRootEnd(drag.id) === null) return
          e.preventDefault()
          e.dataTransfer.dropEffect = 'move'
        }}
        onDrop={(e) => {
          if (drag === null || (e.target as HTMLElement).closest('[role="treeitem"]') !== null)
            return
          finishDrop(e, dropAtRootEnd(drag.id))
        }}
      >
        {renderRows()}
        {nodes.length === 0 && <p className="tree__empty">{t('wiki.tree.empty')}</p>}
      </div>

      {moving !== null && (
        <MoveDialog
          key={moving.key}
          page={movingNode}
          open={moving.open}
          nodes={nodes}
          onClose={() => {
            setMoving((m) => (m === null ? null : { ...m, open: false }))
          }}
          onPick={(parentId) => {
            setMoving((m) => (m === null ? null : { ...m, open: false }))
            if (movingNode === null) return
            if (parentId !== null) expand(parentId)
            const end = childrenOf(nodes, parentId).filter((n) => n.id !== movingNode.id).length
            void actions.move(movingNode.id, parentId, end)
          }}
        />
      )}

      {deleting !== null && (
        <Dialog
          title={t('wiki.delete.title', {
            name: deleting.page.title === '' ? t('wiki.untitled') : deleting.page.title,
          })}
          open={deleting.open}
          closeLabel={t('ui.dialog.cancel')}
          onClose={() => {
            setDeleting((d) => (d === null ? null : { ...d, open: false }))
          }}
          actions={
            <Button
              variant="danger"
              fullWidth={false}
              onClick={() => {
                setDeleting((d) => (d === null ? null : { ...d, open: false }))
                void actions.remove(deleting.page.id)
              }}
            >
              {t('wiki.tree.delete')}
            </Button>
          }
        >
          <p>
            {deleteCount > 0
              ? t('wiki.delete.confirmChildren', { count: deleteCount })
              : t('wiki.delete.confirm')}
          </p>
        </Dialog>
      )}
    </div>
  )
}
