import { useTranslation } from 'react-i18next'
import { FitIcon, MinusIcon, PlusIcon } from './icons'

interface ZoomControlsProps {
  zoom: number
  onZoomIn: () => void
  onZoomOut: () => void
  onFit: () => void
}

/** ZoomControls sit in the bottom-right corner of the canvas: "−", percent, "+", fit. */
export function ZoomControls({ zoom, onZoomIn, onZoomOut, onFit }: ZoomControlsProps) {
  const { t } = useTranslation()
  return (
    <div className="zoom">
      <button
        type="button"
        className="zoom__button"
        aria-label={t('table.zoom.out')}
        onClick={onZoomOut}
      >
        <MinusIcon />
      </button>
      <span className="zoom__value">{`${String(Math.round(zoom * 100))}%`}</span>
      <button
        type="button"
        className="zoom__button"
        aria-label={t('table.zoom.in')}
        onClick={onZoomIn}
      >
        <PlusIcon />
      </button>
      <button
        type="button"
        className="zoom__button"
        aria-label={t('table.zoom.fit')}
        onClick={onFit}
      >
        <FitIcon />
      </button>
    </div>
  )
}
