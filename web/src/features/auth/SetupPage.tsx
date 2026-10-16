import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { navigate } from '../../app/router'
import { useSession } from '../../app/session'
import { postSetup } from '../../shared/api/auth'
import { errorKey } from '../../shared/api/errors'
import { isValidLogin, isValidPassword, normalizeLogin } from '../../shared/validation'
import { AuthLayout } from '../../ui/AuthLayout'
import { Button } from '../../ui/Button'
import { ErrorNote } from '../../ui/ErrorNote'
import { Field } from '../../ui/Field'

interface FieldErrors {
  login?: string
  password?: string
  repeat?: string
}

/** SetupPage creates the GM account on the first run. */
export function SetupPage() {
  const { t } = useTranslation()
  const { setMe } = useSession()
  const setupToken = new URLSearchParams(window.location.search).get('t')

  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [repeat, setRepeat] = useState('')
  const [errors, setErrors] = useState<FieldErrors>({})
  const [failure, setFailure] = useState<string | null>(null)
  const [pending, setPending] = useState(false)

  function submit(e: React.SyntheticEvent) {
    e.preventDefault()
    if (setupToken === null) return

    const found: FieldErrors = {}
    if (!isValidLogin(login)) found.login = t('validation.login')
    if (!isValidPassword(password)) found.password = t('validation.password')
    else if (repeat !== password) found.repeat = t('validation.passwordMismatch')
    setErrors(found)
    if (Object.keys(found).length > 0) return

    setFailure(null)
    setPending(true)
    postSetup(setupToken, normalizeLogin(login), password).then(
      (me) => {
        setMe(me)
        navigate('/', { replace: true })
      },
      (err: unknown) => {
        setFailure(errorKey(err))
        setPending(false)
      },
    )
  }

  return (
    <AuthLayout context={t('setup.context')} title={t('setup.title')}>
      {setupToken === null ? (
        <ErrorNote>{t('setup.noToken')}</ErrorNote>
      ) : (
        <>
          <p>{t('setup.lead')}</p>
          <form onSubmit={submit} noValidate>
            <Field
              id="setup-login"
              label={t('common.login')}
              value={login}
              onChange={setLogin}
              error={errors.login}
              autoComplete="username"
            />
            <Field
              id="setup-password"
              label={t('common.password')}
              type="password"
              value={password}
              onChange={setPassword}
              error={errors.password}
              autoComplete="new-password"
            />
            <Field
              id="setup-repeat"
              label={t('common.passwordRepeat')}
              type="password"
              value={repeat}
              onChange={setRepeat}
              error={errors.repeat}
              autoComplete="new-password"
            />
            {failure !== null && <ErrorNote>{t(failure)}</ErrorNote>}
            <Button type="submit" disabled={pending}>
              {t('setup.submit')}
            </Button>
          </form>
        </>
      )}
    </AuthLayout>
  )
}
