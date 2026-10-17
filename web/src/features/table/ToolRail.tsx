import { useTranslation } from 'react-i18next'
import { SelectIcon } from './icons'

/** ToolRail is the tool strip. For now it has one tool: select and move tokens (the map pans by dragging an empty place). */
export function ToolRail() {
  const { t } = useTranslation()
  return (
    <div
      className="rail"
      role="toolbar"
      aria-orientation="vertical"
      aria-label={t('table.tool.select')}
    >
      <button
        type="button"
        className="rail__button"
        aria-label={t('table.tool.select')}
        title={t('table.tool.select')}
        aria-pressed="true"
        aria-keyshortcuts="V"
      >
        <SelectIcon />
      </button>
    </div>
  )
}
