import { useTranslation } from 'react-i18next'
import { HideFogIcon, RevealFogIcon, SelectIcon } from './icons'

/** TableTool is the tool in hand: select and move tokens, or one of the GM's fog tools. */
export type TableTool = 'select' | 'reveal' | 'hide'

interface ToolRailProps {
  tool: TableTool
  onTool: (tool: TableTool) => void
  isGm: boolean
}

/** ToolRail is the tool strip: select and move tokens (the map pans by dragging an empty place), and below the GM's tools. */
export function ToolRail({ tool, onTool, isGm }: ToolRailProps) {
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
        title={`${t('table.tool.select')} (V)`}
        aria-pressed={tool === 'select'}
        aria-keyshortcuts="V"
        onClick={() => {
          onTool('select')
        }}
      >
        <SelectIcon />
      </button>
      {isGm && (
        <>
          <hr className="rail__separator" />
          <button
            type="button"
            className="rail__button rail__button--gm"
            aria-label={t('fog.tool.reveal')}
            title={`${t('fog.tool.reveal')} (F)`}
            aria-pressed={tool === 'reveal'}
            aria-keyshortcuts="F"
            onClick={() => {
              onTool(tool === 'reveal' ? 'select' : 'reveal')
            }}
          >
            <RevealFogIcon />
          </button>
          <button
            type="button"
            className="rail__button rail__button--gm"
            aria-label={t('fog.tool.hide')}
            title={`${t('fog.tool.hide')} (Shift+F)`}
            aria-pressed={tool === 'hide'}
            aria-keyshortcuts="Shift+F"
            onClick={() => {
              onTool(tool === 'hide' ? 'select' : 'hide')
            }}
          >
            <HideFogIcon />
          </button>
        </>
      )}
    </div>
  )
}
