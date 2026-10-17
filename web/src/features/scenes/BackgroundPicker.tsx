import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { listAssets, uploadAsset, type Asset } from '../../shared/api/assets'
import { errorKey } from '../../shared/api/errors'
import { Button } from '../../ui/Button'
import { ErrorNote } from '../../ui/ErrorNote'

interface BackgroundPickerProps {
  campaignId: string
  /** value is the id of the chosen asset; null: no background. */
  value: string | null
  onChange: (assetId: string | null) => void
  /** onAssets reports the images of the campaign, so the form can read the size of the chosen one. */
  onAssets: (assets: Asset[]) => void
}

const accepted = 'image/png,image/jpeg,image/webp,image/gif'

/** BackgroundPicker chooses the background among the uploaded images or uploads a new one. */
export function BackgroundPicker({ campaignId, value, onChange, onAssets }: BackgroundPickerProps) {
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
        const images = list.filter((a) => a.mime.startsWith('image/'))
        setAssets(images)
        report.current(images)
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
  }, [campaignId])

  async function upload(file: File) {
    setError(null)
    setProgress(0)
    try {
      const asset = await uploadAsset(campaignId, file, 'map', setProgress)
      const next = [asset, ...(assets ?? []).filter((a) => a.id !== asset.id)]
      setAssets(next)
      report.current(next)
      onChange(asset.id)
    } catch (err) {
      setError(errorKey(err))
    } finally {
      setProgress(null)
    }
  }

  const percent = progress === null ? 0 : Math.round(progress * 100)

  return (
    <div className="bg-picker">
      {assets !== null && assets.length === 0 && progress === null && (
        <p className="bg-picker__empty">{t('scenes.bg.empty')}</p>
      )}
      {assets !== null && assets.length > 0 && (
        <ul className="bg-picker__grid">
          {assets.map((a) => (
            <li key={a.id}>
              <button
                type="button"
                className="bg-picker__item"
                aria-pressed={a.id === value}
                title={a.originalName}
                onClick={() => {
                  onChange(a.id)
                }}
              >
                <img src={a.url} alt="" loading="lazy" width={96} height={64} />
                <span className="bg-picker__name">{a.originalName}</span>
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className="bg-picker__actions">
        <Button
          variant="secondary"
          fullWidth={false}
          disabled={progress !== null}
          onClick={() => {
            fileInput.current?.click()
          }}
        >
          {t('scenes.bg.upload')}
        </Button>
        <Button
          variant="secondary"
          fullWidth={false}
          disabled={value === null}
          onClick={() => {
            onChange(null)
          }}
        >
          {t('scenes.bg.none')}
        </Button>
        <input
          ref={fileInput}
          type="file"
          className="bg-picker__file"
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
        <div className="bg-picker__progress">
          <div
            className="bg-picker__bar"
            role="progressbar"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={percent}
            aria-label={t('scenes.bg.uploading', { percent })}
          >
            <div className="bg-picker__fill" style={{ width: `${String(percent)}%` }} />
          </div>
          <p className="bg-picker__percent" aria-live="polite">
            {t('scenes.bg.uploading', { percent })}
          </p>
        </div>
      )}
      {error !== null && <ErrorNote>{t(error)}</ErrorNote>}
    </div>
  )
}
