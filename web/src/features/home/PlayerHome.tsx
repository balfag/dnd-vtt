import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { navigate } from '../../app/router'
import { useSession } from '../../app/session'
import { AuthLayout } from '../../ui/AuthLayout'
import { Button } from '../../ui/Button'

/** PlayerHome sends a signed-in player to the table; it stays as the fallback screen. */
export function PlayerHome({ name }: { name: string }) {
  const { t } = useTranslation()
  const { logout } = useSession()
  useEffect(() => {
    navigate('/table', { replace: true })
  }, [])
  return (
    <AuthLayout title={t('home.player.title', { name })}>
      <p>{t('home.player.waiting')}</p>
      <Button
        onClick={() => {
          void logout()
        }}
      >
        {t('common.signOut')}
      </Button>
    </AuthLayout>
  )
}
