import { useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'
import { commandErrorKey } from '../../shared/ws/errors'
import { Button } from '../../ui/Button'
import '../scenes/scenes.css'

interface PrepFrameProps {
  /** viewedName is the scene on the GM's canvas. */
  viewedName: string
  /** hasActive: players see a scene (another one). */
  hasActive: boolean
  /** onShow shows the viewed scene to the players. */
  onShow: () => Promise<void>
}

/**
 * PrepFrame marks that the GM looks at a scene the players do not see: a brass frame
 * inside the canvas and a note with the way to show the scene. The caller renders it only then.
 */
export function PrepFrame({ viewedName, hasActive, onShow }: PrepFrameProps) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function show() {
    setBusy(true)
    setError(null)
    try {
      await onShow()
    } catch (err) {
      setError(commandErrorKey(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <div className="prep-frame" aria-hidden="true" />
      <div className="prep-note" role="status">
        <p className="prep-note__text">
          {hasActive ? (
            <Trans
              i18nKey="table.prep.note"
              values={{ name: viewedName }}
              components={{ b: <b /> }}
            />
          ) : (
            t('table.prep.noActive')
          )}
        </p>
        <Button
          fullWidth={false}
          disabled={busy}
          onClick={() => {
            void show()
          }}
        >
          {t('scenes.showToPlayers')}
        </Button>
        {error !== null && (
          <p className="prep-note__error" role="alert">
            {t(error)}
          </p>
        )}
      </div>
    </>
  )
}
