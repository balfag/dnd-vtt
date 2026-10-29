import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { listAssets, uploadAsset, type Asset } from '../../shared/api/assets'
import { errorKey } from '../../shared/api/errors'
import { Button } from '../../ui/Button'
import { ErrorNote } from '../../ui/ErrorNote'
import './assets.css'

interface AssetPickerProps {
  campaignId: string
  /** kind of the images to show and of the images that are uploaded here. */
  kind: 'map' | 'token' | 'other'
  /** value is the id of the chosen asset; null: none. */
  value: string | null
  /** onChange gets the chosen image, or null for the "none" button. */
  onChange: (asset: Asset | null) => void
  /** onAssets reports the images of the campaign, so the form can read the size of the chosen one. */
  onAssets?: (assets: Asset[]) => void
  /** noneLabel is the text of the button that clears the choice. */
  noneLabel: string
}

const accepted = 'image/png,image/jpeg,image/webp,image/gif'

/** AssetPicker chooses an image of the campaign among the uploaded ones or uploads a new one. */
export function AssetPicker({
  campaignId,
  kind,
  value,
  onChange,
  onAssets,
  noneLabel,
}: AssetPickerProps) {
  const { t } = useTranslation()
  const [assets, setAssets] = useState<Asset[] | null>(null)
  const [progress, setProgress] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  // onAssets is called from an effect that must not run again when the parent re-renders.
  const report = useRef(onAssets)
  useEffect(() => {
    report.current = onAssets
  })

  useEffect(() => {
    let cancelled = false
    listAssets(campaignId).then(
      (list) => {
        if (cancelled) return
        const images = list.filter((a) => a.mime.startsWith('image/') && a.kind === kind)
        setAssets(images)
        report.current?.(images)
      },
      (err: unknown) => {
        if (cancelled) return
        setAssets([])
        setError(errorKey(err))
      },
    )
    return () => {
      cancelled = true
    }
  }, [campaignId, kind])

  async function upload(file: File) {
    setError(null)
    setProgress(0)
    try {
      const asset = await uploadAsset(campaignId, file, kind, setProgress)
      const next = [asset, ...(assets ?? []).filter((a) => a.id !== asset.id)]
      setAssets(next)
      report.current?.(next)
      onChange(asset)
    } catch (err) {
      setError(errorKey(err))
    } finally {
      setProgress(null)
    }
  }

  const percent = progress === null ? 0 : Math.round(progress * 100)

  return (
    <div className="asset-picker">
      {assets !== null && assets.length === 0 && progress === null && (
        <p className="asset-picker__empty">{t('assets.empty')}</p>
      )}
      {assets !== null && assets.length > 0 && (
        <ul className="asset-picker__grid">
          {assets.map((a) => (
            <li key={a.id}>
              <button
                type="button"
                className="asset-picker__item"
                aria-pressed={a.id === value}
                title={a.originalName}
                onClick={() => {
                  onChange(a)
                }}
              >
                <img src={a.url} alt="" loading="lazy" width={96} height={64} />
                <span className="asset-picker__name">{a.originalName}</span>
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className="asset-picker__actions">
        <Button
          variant="secondary"
          fullWidth={false}
          disabled={progress !== null}
          onClick={() => {
            fileInput.current?.click()
          }}
        >
          {t('assets.upload')}
        </Button>
        <button
          type="button"
          className="button button--secondary button--inline asset-picker__none"
          aria-pressed={value === null}
          onClick={() => {
            onChange(null)
          }}
        >
          {noneLabel}
        </button>
        <input
          ref={fileInput}
          type="file"
          className="asset-picker__file"
          accept={accepted}
          tabIndex={-1}
          aria-hidden="true"
          onChange={(e) => {
            const file = e.target.files?.[0]
            e.target.value = ''
            if (file !== undefined) void upload(file)
          }}
        />
      </div>

      {progress !== null && (
        <div className="asset-picker__progress">
          <div
            className="asset-picker__bar"
            role="progressbar"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={percent}
            aria-label={t('assets.uploading', { percent })}
          >
            <div className="asset-picker__fill" style={{ width: `${String(percent)}%` }} />
          </div>
          <p className="asset-picker__percent" aria-live="polite">
            {t('assets.uploading', { percent })}
          </p>
        </div>
      )}
      {error !== null && <ErrorNote>{t(error)}</ErrorNote>}
    </div>
  )
}
