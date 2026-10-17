import { useTranslation } from 'react-i18next'
import { PanIcon } from './icons'

/** ToolRail is the tool strip. For now it has one tool: moving the map. */
export function ToolRail() {
  const { t } = useTranslation()
  return (
    <div
      className="rail"
      role="toolbar"
      aria-orientation="vertical"
      aria-label={t('table.tool.pan')}
    >
      <button
        type="button"
        className="rail__button"
        aria-label={t('table.tool.pan')}
        title={t('table.tool.pan')}
        aria-pressed="true"
      >
        <PanIcon />
      </button>
    </div>
  )
}
