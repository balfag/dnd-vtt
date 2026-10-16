import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { navigate } from '../../app/router'
import { useSession } from '../../app/session'
import { getGameLink, postLogin, postRegister } from '../../shared/api/auth'
import { ApiError } from '../../shared/api/client'
import { errorKey } from '../../shared/api/errors'
import {
  isValidDisplayName,
  isValidLogin,
  isValidPassword,
  normalizeLogin,
} from '../../shared/validation'
import { AuthLayout } from '../../ui/AuthLayout'
import { Button } from '../../ui/Button'
import { ErrorNote } from '../../ui/ErrorNote'
import { Field } from '../../ui/Field'
import { panelId, tabId } from '../../ui/tabIds'
import { Tabs } from '../../ui/Tabs'

type Link =
  | { status: 'loading' }
  | { status: 'dead' }
  | { status: 'failed'; key: string }
  | { status: 'ok'; campaignName: string }

/** PlayPage is the entrance of a player: the game link leads to sign-in or registration. */
export function PlayPage({ token }: { token: string }) {
  const { t } = useTranslation()
  const [link, setLink] = useState<Link>({ status: 'loading' })

  useEffect(() => {
    let cancelled = false
    getGameLink(token).then(
      ({ campaignName }) => {
        if (!cancelled) setLink({ status: 'ok', campaignName })
      },
      (err: unknown) => {
        if (cancelled) return
        if (err instanceof ApiError && err.status === 403) setLink({ status: 'dead' })
        else setLink({ status: 'failed', key: errorKey(err) })
      },
    )
    return () => {
      cancelled = true
    }
  }, [token])

  if (link.status === 'loading') {
    return <main className="auth" aria-busy="true" />
  }
  if (link.status === 'dead') {
    return (
      <AuthLayout title={t('play.linkDeadTitle')}>
        <p>{t('play.linkDead')}</p>
      </AuthLayout>
    )
  }
  if (link.status === 'failed') {
    return (
      <AuthLayout title={t('play.title')}>
        <ErrorNote>{t(link.key)}</ErrorNote>
      </AuthLayout>
    )
  }
  return (
    <AuthLayout context={link.campaignName} title={t('play.title')}>
      <PlayForms token={token} />
    </AuthLayout>
  )
}

type Mode = 'signIn' | 'register'

function PlayForms({ token }: { token: string }) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<Mode>('signIn')
  const tabs = [
    { id: 'signIn', label: t('play.tabSignIn') },
    { id: 'register', label: t('play.tabRegister') },
  ]
  return (
    <>
      <Tabs
        tabs={tabs}
        selected={mode}
        onSelect={(id) => {
          setMode(id === 'register' ? 'register' : 'signIn')
        }}
      />
      <div role="tabpanel" id={panelId(mode)} aria-labelledby={tabId(mode)}>
        {mode === 'signIn' ? <SignInForm token={token} /> : <RegisterForm token={token} />}
      </div>
    </>
  )
}

function SignInForm({ token }: { token: string }) {
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
    postLogin(normalizeLogin(login), password, token).then(
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
    <form onSubmit={submit}>
      <Field
        id="play-login"
        label={t('common.login')}
        value={login}
        onChange={setLogin}
        autoComplete="username"
      />
      <Field
        id="play-password"
        label={t('common.password')}
        type="password"
        value={password}
        onChange={setPassword}
        hint={t('play.forgot')}
        autoComplete="current-password"
      />
      {failure !== null && <ErrorNote>{t(failure)}</ErrorNote>}
      <Button type="submit" disabled={pending}>
        {t('play.submitSignIn')}
      </Button>
    </form>
  )
}

interface RegisterErrors {
  login?: string
  displayName?: string
  password?: string
  repeat?: string
}

function RegisterForm({ token }: { token: string }) {
  const { t } = useTranslation()
  const { setMe } = useSession()
  const [login, setLogin] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [repeat, setRepeat] = useState('')
  const [errors, setErrors] = useState<RegisterErrors>({})
  const [failure, setFailure] = useState<string | null>(null)
  const [pending, setPending] = useState(false)

  function submit(e: React.SyntheticEvent) {
    e.preventDefault()
    const found: RegisterErrors = {}
    if (!isValidLogin(login)) found.login = t('validation.login')
    if (!isValidDisplayName(displayName)) found.displayName = t('validation.displayName')
    if (!isValidPassword(password)) found.password = t('validation.password')
    else if (repeat !== password) found.repeat = t('validation.passwordMismatch')
    setErrors(found)
    if (Object.keys(found).length > 0) return

    setFailure(null)
    setPending(true)
    postRegister(token, normalizeLogin(login), password, displayName.trim()).then(
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
    <form onSubmit={submit} noValidate>
      <Field
        id="register-login"
        label={t('common.login')}
        value={login}
        onChange={setLogin}
        error={errors.login}
        autoComplete="username"
      />
      <Field
        id="register-name"
        label={t('common.displayName')}
        value={displayName}
        onChange={setDisplayName}
        error={errors.displayName}
        autoComplete="nickname"
      />
      <Field
        id="register-password"
        label={t('common.password')}
        type="password"
        value={password}
        onChange={setPassword}
        error={errors.password}
        autoComplete="new-password"
      />
      <Field
        id="register-repeat"
        label={t('common.passwordRepeat')}
        type="password"
        value={repeat}
        onChange={setRepeat}
        error={errors.repeat}
        autoComplete="new-password"
      />
      {failure !== null && <ErrorNote>{t(failure)}</ErrorNote>}
      <Button type="submit" disabled={pending}>
        {t('play.submitRegister')}
      </Button>
    </form>
  )
}
