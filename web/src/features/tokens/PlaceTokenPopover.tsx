import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import type { Asset } from '../../shared/api/assets'
import { AssetPicker } from '../assets/AssetPicker'
import './tokens.css'

interface PlaceTokenPopoverProps {
  campaignId: string
  /** assetId is the image the next token gets; null: none. */
  assetId: string | null
  onChange: (asset: Asset | null) => void
  onClose: () => void
}

/** PlaceTokenPopover is next to the tool strip while the place tool is in hand: it picks the image of the tokens to place. */
export function PlaceTokenPopover({
  campaignId,
  assetId,
  onChange,
  onClose,
}: PlaceTokenPopoverProps) {
  const { t } = useTranslation()
  const root = useRef<HTMLDivElement>(null)

  // The keyboard user lands in the popover when the tool is chosen.
  useEffect(() => {
    root.current?.querySelector<HTMLElement>('button')?.focus()
  }, [])

  return (
    <div
      ref={root}
      className="place-popover"
      role="group"
      aria-label={t('tokens.place.label')}
      onKeyDown={(e) => {
        if (e.key !== 'Escape') return
        e.preventDefault()
        e.stopPropagation()
        onClose()
      }}
    >
      <AssetPicker
        campaignId={campaignId}
        kind="token"
        value={assetId}
        noneLabel={t('tokens.place.noImage')}
        onChange={onChange}
      />
    </div>
  )
}
