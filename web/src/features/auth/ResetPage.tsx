import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { postPasswordReset } from '../../shared/api/auth'
import { ApiError } from '../../shared/api/client'
import { errorKey } from '../../shared/api/errors'
import { isValidPassword } from '../../shared/validation'
import { AuthLayout } from '../../ui/AuthLayout'
import { Button } from '../../ui/Button'
import { ErrorNote } from '../../ui/ErrorNote'
import { Field } from '../../ui/Field'

/** ResetPage sets a new password by a one-time link from the GM. */
export function ResetPage({ token }: { token: string }) {
  const { t } = useTranslation()
  const [password, setPassword] = useState('')
  const [repeat, setRepeat] = useState('')
  const [passwordError, setPasswordError] = useState<string>()
  const [repeatError, setRepeatError] = useState<string>()
  const [failure, setFailure] = useState<string | null>(null)
  const [pending, setPending] = useState(false)
  const [done, setDone] = useState(false)

  function submit(e: React.SyntheticEvent) {
    e.preventDefault()
    const tooShort = !isValidPassword(password)
    const mismatch = !tooShort && repeat !== password
    setPasswordError(tooShort ? t('validation.password') : undefined)
    setRepeatError(mismatch ? t('validation.passwordMismatch') : undefined)
    if (tooShort || mismatch) return

    setFailure(null)
    setPending(true)
    postPasswordReset(token, password).then(
      () => {
        setDone(true)
      },
      (err: unknown) => {
        setFailure(err instanceof ApiError && err.status === 403 ? 'reset.linkDead' : errorKey(err))
        setPending(false)
      },
    )
  }

  if (done) {
    return (
      <AuthLayout title={t('reset.title')}>
        <p>{t('reset.done')}</p>
      </AuthLayout>
    )
  }

  return (
    <AuthLayout title={t('reset.title')}>
      <form onSubmit={submit} noValidate>
        <Field
          id="reset-password"
          label={t('common.password')}
          type="password"
          value={password}
          onChange={setPassword}
          error={passwordError}
          autoComplete="new-password"
        />
        <Field
          id="reset-repeat"
          label={t('common.passwordRepeat')}
          type="password"
          value={repeat}
          onChange={setRepeat}
          error={repeatError}
          autoComplete="new-password"
        />
        {failure !== null && <ErrorNote>{t(failure)}</ErrorNote>}
        <Button type="submit" disabled={pending}>
          {t('reset.submit')}
        </Button>
      </form>
    </AuthLayout>
  )
}
