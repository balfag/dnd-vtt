import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { PageNode } from '../../../shared/api/wiki'
import { Button } from '../../../ui/Button'
import { Dialog } from '../../../ui/Dialog'
import { parentCandidates } from './tree'

interface MoveDialogProps {
  /** page is the page to move; null keeps the dialog closed. */
  page: PageNode | null
  open: boolean
  nodes: PageNode[]
  /** onPick gets the new parent: a page id, or null for the root. */
  onPick: (parentId: string | null) => void
  onClose: () => void
}

/** MoveDialog moves a page with the keyboard: it finds the new parent by name instead of dragging. */
export function MoveDialog({ page, open, nodes, onPick, onClose }: MoveDialogProps) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const candidates = page === null ? [] : parentCandidates(nodes, page.id, query)
  const name = page === null ? '' : page.title === '' ? t('wiki.untitled') : page.title

  return (
    <Dialog
      title={t('wiki.move.title', { name })}
      open={open}
      closeLabel={t('ui.dialog.cancel')}
      onClose={onClose}
      actions={
        <Button
          fullWidth={false}
          onClick={() => {
            onPick(null)
          }}
        >
          {t('wiki.move.root')}
        </Button>
      }
    >
      <div className="move">
        <input
          className="field__input"
          type="search"
          aria-label={t('wiki.move.search')}
          placeholder={t('wiki.move.search')}
          value={query}
          autoFocus
          onChange={(e) => {
            setQuery(e.target.value)
          }}
        />
        {candidates.length === 0 ? (
          <p className="move__empty">{t('wiki.move.empty')}</p>
        ) : (
          <ul className="move__list">
            {candidates.map((c) => (
              <li key={c.id}>
                <button
                  type="button"
                  className="move__item"
                  onClick={() => {
                    onPick(c.id)
                  }}
                >
                  <span aria-hidden="true">{c.icon !== '' ? c.icon : '◦'}</span>
                  <span>{c.title === '' ? t('wiki.untitled') : c.title}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Dialog>
  )
}
