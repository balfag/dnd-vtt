import { useTranslation } from 'react-i18next'
import { useSession } from '../../app/session'
import { AuthLayout } from '../../ui/AuthLayout'
import { Button } from '../../ui/Button'

/** GmHome is the GM's screen right after sign-in, until the campaigns screen exists. */
export function GmHome({ login }: { login: string }) {
  const { t } = useTranslation()
  const { logout } = useSession()
  return (
    <AuthLayout title={t('home.gm.title', { login })}>
      <p>{t('home.gm.next')}</p>
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
