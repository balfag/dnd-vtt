import { useTranslation } from 'react-i18next'
import { useSession } from '../../app/session'
import { AuthLayout } from '../../ui/AuthLayout'
import { Button } from '../../ui/Button'

/** PlayerHome is the player's screen after sign-in, until the table exists. */
export function PlayerHome({ name }: { name: string }) {
  const { t } = useTranslation()
  const { logout } = useSession()
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
