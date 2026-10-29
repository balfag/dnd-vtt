import { useTranslation } from 'react-i18next'
import {
  DrawIcon,
  HideFogIcon,
  PingIcon,
  PlaceTokenIcon,
  RevealFogIcon,
  RulerIcon,
  SelectIcon,
  TemplateIcon,
} from './icons'

/** TableTool is the tool in hand: select and move; the tools of everyone (ruler, ping, draw, template); the GM's tools (place tokens, reveal or hide fog). */
export type TableTool =
  'select' | 'ruler' | 'ping' | 'draw' | 'template' | 'place' | 'reveal' | 'hide'

interface ToolRailProps {
  tool: TableTool
  onTool: (tool: TableTool) => void
  isGm: boolean
}

interface RailButton {
  tool: TableTool
  label: string
  key: string
  icon: React.ReactNode
}

const sharedTools: RailButton[] = [
  { tool: 'ruler', label: 'tools.ruler.name', key: 'R', icon: <RulerIcon /> },
  { tool: 'ping', label: 'tools.ping.name', key: 'P', icon: <PingIcon /> },
  { tool: 'draw', label: 'tools.draw.name', key: 'D', icon: <DrawIcon /> },
  { tool: 'template', label: 'tools.template.name', key: 'T', icon: <TemplateIcon /> },
]

const gmButtons: RailButton[] = [
  { tool: 'place', label: 'tokens.tool.place', key: 'K', icon: <PlaceTokenIcon /> },
  { tool: 'reveal', label: 'fog.tool.reveal', key: 'F', icon: <RevealFogIcon /> },
  { tool: 'hide', label: 'fog.tool.hide', key: 'Shift+F', icon: <HideFogIcon /> },
]

/** ToolRail is the tool strip: select and move tokens (the map pans by dragging an empty place), the tools of everyone, and below the GM's tools. */
export function ToolRail({ tool, onTool, isGm }: ToolRailProps) {
  const { t } = useTranslation()

  function button(b: RailButton, gm: boolean) {
    return (
      <button
        key={b.tool}
        type="button"
        className={gm ? 'rail__button rail__button--gm' : 'rail__button'}
        data-tool={b.tool}
        aria-label={t(b.label)}
        title={`${t(b.label)} (${b.key})`}
        aria-pressed={tool === b.tool}
        aria-keyshortcuts={b.key}
        onClick={() => {
          onTool(tool === b.tool ? 'select' : b.tool)
        }}
      >
        {b.icon}
      </button>
    )
  }

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
        data-tool="select"
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
      {sharedTools.map((b) => button(b, false))}
      {isGm && (
        <>
          <hr className="rail__separator" />
          {gmButtons.map((b) => button(b, true))}
        </>
      )}
    </div>
  )
}
