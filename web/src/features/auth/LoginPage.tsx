import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { navigate } from '../../app/router'
import { useSession } from '../../app/session'
import { postLogin } from '../../shared/api/auth'
import { errorKey } from '../../shared/api/errors'
import { normalizeLogin } from '../../shared/validation'
import { AuthLayout } from '../../ui/AuthLayout'
import { Button } from '../../ui/Button'
import { ErrorNote } from '../../ui/ErrorNote'
import { Field } from '../../ui/Field'

/** LoginPage signs the GM in with a password. */
export function LoginPage() {
  const { t } = useTranslation()
  const { setMe } = useSession()
  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [failure, setFailure] = useState<string | null>(null)
  const [pending, setPending] = useState(false)

  function submit(e: React.SyntheticEvent) {
    e.preventDefault()
    setFailure(null)
    setPending(true)
    postLogin(normalizeLogin(login), password).then(
      (me) => {
        setMe(me)
        navigate('/')
      },
      (err: unknown) => {
        setFailure(errorKey(err))
        setPending(false)
      },
    )
  }

  return (
    <AuthLayout title={t('login.title')}>
      <form onSubmit={submit}>
        <Field
          id="login-login"
          label={t('common.login')}
          value={login}
          onChange={setLogin}
          autoComplete="username"
        />
        <Field
          id="login-password"
          label={t('common.password')}
          type="password"
          value={password}
          onChange={setPassword}
          autoComplete="current-password"
        />
        {failure !== null && <ErrorNote>{t(failure)}</ErrorNote>}
        <Button type="submit" disabled={pending}>
          {t('login.submit')}
        </Button>
      </form>
      <p style={{ marginTop: 16 }}>{t('login.playersHint')}</p>
    </AuthLayout>
  )
}
