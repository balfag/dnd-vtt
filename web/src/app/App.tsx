import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LoginPage } from '../features/auth/LoginPage'
import { PlayPage } from '../features/auth/PlayPage'
import { ResetPage } from '../features/auth/ResetPage'
import { SetupPage } from '../features/auth/SetupPage'
import { GmHome } from '../features/home/GmHome'
import { PlayerHome } from '../features/home/PlayerHome'
import { getSetupNeeded, postGMToken } from '../shared/api/auth'
import { AuthLayout } from '../ui/AuthLayout'
import { Link } from '../ui/Link'
import { ErrorNote } from '../ui/ErrorNote'
import { navigate, useRoute } from './router'
import { SessionProvider, useSession } from './session'

/** App picks the page by route inside the session provider. */
export function App() {
  return (
    <SessionProvider>
      <Pages />
    </SessionProvider>
  )
}

function Pages() {
  const route = useRoute()
  switch (route.name) {
    case 'home':
      return <HomeRoute />
    case 'setup':
      return <SetupPage />
    case 'login':
      return <LoginPage />
    case 'play':
      return <PlayPage token={route.token} />
    case 'reset':
      return <ResetPage token={route.token} />
    case 'notFound':
      return <NotFound />
  }
}

function NotFound() {
  const { t } = useTranslation()
  return (
    <AuthLayout title={t('notFound.title')}>
      <p>
        <Link to="/">{t('notFound.home')}</Link>
      </p>
    </AuthLayout>
  )
}

/**
 * HomeRoute is "/": it redeems a GM link (?gm=), sends an anonymous visitor to
 * setup or sign-in, and shows the home screen of whoever is signed in.
 */
function HomeRoute() {
  const { t } = useTranslation()
  const { state, setMe } = useSession()
  const [gmLink] = useState(() => new URLSearchParams(window.location.search).get('gm'))
  const [gmLinkFailed, setGmLinkFailed] = useState(false)
  // The GM token is one-time and effects run twice in StrictMode: redeem it once.
  const gmStarted = useRef(false)
  const gmFinished = useRef(false)
  const redirected = useRef(false)

  useEffect(() => {
    if (state.status === 'loading') return

    if (gmLink !== null && !gmStarted.current) {
      gmStarted.current = true
      postGMToken(gmLink).then(
        (me) => {
          gmFinished.current = true
          setMe(me)
          navigate('/', { replace: true })
        },
        () => {
          gmFinished.current = true
          setGmLinkFailed(true)
          navigate('/', { replace: true })
        },
      )
      return
    }
    if (gmLink !== null && !gmFinished.current) return

    if (state.status === 'anonymous' && !redirected.current) {
      redirected.current = true
      getSetupNeeded().then(
        (needed) => {
          navigate(needed ? '/setup' : '/login', { replace: true })
        },
        () => {
          navigate('/login', { replace: true })
        },
      )
    }
  }, [state.status, gmLink, setMe])

  if (gmLinkFailed) {
    return (
      <AuthLayout title={t('login.title')}>
        <ErrorNote>{t('home.gmLinkExpired')}</ErrorNote>
        <p style={{ marginTop: 16 }}>
          <Link to="/login">{t('home.gmLinkLogin')}</Link>
        </p>
      </AuthLayout>
    )
  }
  if (state.status !== 'signedIn') {
    return null
  }
  return state.me.isGm ? (
    <GmHome login={state.me.login} />
  ) : (
    <PlayerHome name={state.me.player?.displayName ?? state.me.login} />
  )
}
