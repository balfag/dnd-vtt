import { useTranslation } from 'react-i18next'
import type { SceneView } from '../../shared/protocol'
import { cellAt } from './canvas/grid'

/**
 * TokenList is the tokens of the scene as the viewer sees them, for screen readers: the canvas
 * has no text of its own. It is hidden from the eye and follows the store on every change.
 */
export function TokenList({ scene }: { scene: SceneView | null }) {
  const { t } = useTranslation()
  if (scene === null) return null
  const tokens = [...scene.tokens].sort((a, b) => a.z - b.z || a.id.localeCompare(b.id))
  return (
    <ul aria-label={t('table.tokens.label')} className="sr-only">
      {tokens.map((tk) => {
        const c = cellAt(scene.grid, { x: tk.x, y: tk.y })
        const cell = t('table.tokens.cell', { col: c.col + 1, row: c.row + 1 })
        const name = tk.name === '' ? t('table.tokens.unnamed') : tk.name
        return (
          <li key={tk.id} data-token-id={tk.id}>
            {t('table.tokens.item', { name, cell, interpolation: { escapeValue: false } })}
          </li>
        )
      })}
    </ul>
  )
}
