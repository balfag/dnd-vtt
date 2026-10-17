import { useTranslation } from 'react-i18next'
import type { Asset } from '../../shared/api/assets'
import { AssetPicker } from '../assets/AssetPicker'

interface BackgroundPickerProps {
  campaignId: string
  /** value is the id of the chosen asset; null: no background. */
  value: string | null
  onChange: (assetId: string | null) => void
  /** onAssets reports the images of the campaign, so the form can read the size of the chosen one. */
  onAssets: (assets: Asset[]) => void
}

/** BackgroundPicker chooses the background among the uploaded images or uploads a new one. */
export function BackgroundPicker({ campaignId, value, onChange, onAssets }: BackgroundPickerProps) {
  const { t } = useTranslation()
  return (
    <AssetPicker
      campaignId={campaignId}
      kind="map"
      value={value}
      noneLabel={t('scenes.bg.none')}
      onChange={(asset) => {
        onChange(asset?.id ?? null)
      }}
      onAssets={onAssets}
    />
  )
}
