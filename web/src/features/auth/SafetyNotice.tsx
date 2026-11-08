import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '../../ui/Button'
import './auth.css'

const readMoreUrl = 'https://github.com/balfag/dnd-vtt#безопасность-для-игроков'

/** SafetyNotice is the step before the sign-in forms: it shows where the player is connecting. */
export function SafetyNotice({ onAccept }: { onAccept: () => void }) {
  const { t } = useTranslation()
  const accept = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    accept.current?.focus()
  }, [])

  return (
    <div className="safety">
      <p>{t('play.safety.connecting')}</p>
      <p className="safety__address">{window.location.host}</p>
      <p>{t('play.safety.never')}</p>
      <ul className="safety__never">
        <li>{t('play.safety.neverDownload')}</li>
        <li>{t('play.safety.neverPasswords')}</li>
      </ul>
      <p>{t('play.safety.passwordHere')}</p>
      <p>{t('play.safety.trust')}</p>
      <Button ref={accept} onClick={onAccept}>
        {t('play.safety.accept')}
      </Button>
      <p className="safety__note">{t('play.safety.certificate')}</p>
      <p className="safety__note">{t('play.safety.missing')}</p>
      <p className="safety__note">
        {t('play.safety.readMore')}{' '}
        <a href={readMoreUrl} target="_blank" rel="noreferrer">
          {readMoreUrl}
        </a>
      </p>
    </div>
  )
}
