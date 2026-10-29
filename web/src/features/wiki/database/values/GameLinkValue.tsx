import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { searchMentions, type GameLinkValue, type MentionHit } from '../../../../shared/api/wiki'
import { debounceSearch } from '../../page/mentions'
import { gameLabel, gameLinksOf, limits, rememberGameLabel } from '../model'
import { useValueEnv } from './env'
import { PickerPopup } from './PickerPopup'

const SEARCH_DEBOUNCE_MS = 200

function linkKey(l: GameLinkValue): string {
  return `${l.kind}:${l.id}`
}

/** GameLinkView lists the scenes and tokens a value points at; a name the client has not seen is shown as the kind. */
export function GameLinkView({ value }: { value: unknown }) {
  const { t } = useTranslation()
  const links = gameLinksOf(value)
  if (links.length === 0) return null
  return (
    <span className="dbv__list">
      {links.map((l) => (
        <span key={linkKey(l)} className="dbv__game">
          <span className="dbv__kind">{t(`wiki.db.game.${l.kind}`)}</span>{' '}
          {gameLabel(l.kind, l.id) ?? ''}
        </span>
      ))}
    </span>
  )
}

interface GameLinkEditorProps {
  value: unknown
  anchor: HTMLElement
  label: string
  onChange: (next: unknown) => void
  onEnd: () => void
}

/** GameLinkEditor searches the scenes and tokens of the campaign by name. */
export function GameLinkEditor({ value, anchor, label, onChange, onEnd }: GameLinkEditorProps) {
  const { t } = useTranslation()
  const env = useValueEnv()
  const [query, setQuery] = useState('')
  const [hits, setHits] = useState<MentionHit[] | null>(null)
  const selected = gameLinksOf(value)
  const search = useMemo(
    () => debounceSearch((q: string) => searchMentions(env.campaignId, q), SEARCH_DEBOUNCE_MS),
    [env.campaignId],
  )

  useEffect(() => {
    let stale = false
    search(query.trim()).then(
      (list) => {
        if (stale) return
        const games = list.filter((h) => h.kind === 'scene' || h.kind === 'token')
        for (const h of games) rememberGameLabel(h.kind, h.id, h.label)
        setHits(games)
      },
      () => {
        if (!stale) setHits([])
      },
    )
    return () => {
      stale = true
    }
  }, [search, query])

  const q = query.trim().toLocaleLowerCase()
  const chosen = selected
    .map((l) => ({
      id: linkKey(l),
      text: gameLabel(l.kind, l.id) ?? t(`wiki.db.game.${l.kind}`),
      kind: l.kind,
      detail: '',
    }))
    .filter((i) => i.text.toLocaleLowerCase().includes(q))
  const keys = new Set(selected.map(linkKey))
  const found = (hits ?? [])
    .filter((h) => !keys.has(`${h.kind}:${h.id}`))
    .map((h) => ({ id: `${h.kind}:${h.id}`, text: h.label, kind: h.kind, detail: h.detail }))
  const items = [...chosen, ...found].map((i) => ({
    id: i.id,
    text: i.text,
    selected: keys.has(i.id),
    label: (
      <span className="dbpick__text">
        <span className="dbv__kind">{t(`wiki.db.game.${i.kind}`)}</span> {i.text}
        {i.detail !== '' && <span className="dbpick__detail"> · {i.detail}</span>}
      </span>
    ),
  }))

  function pick(id: string) {
    const at = id.indexOf(':')
    const link: GameLinkValue = {
      kind: id.slice(0, at) === 'token' ? 'token' : 'scene',
      id: id.slice(at + 1),
    }
    const next = keys.has(id) ? selected.filter((l) => linkKey(l) !== id) : [...selected, link]
    onChange(next.length === 0 ? null : next)
  }

  return (
    <PickerPopup
      anchor={anchor}
      label={label}
      items={items}
      loading={hits === null}
      query={query}
      onQuery={setQuery}
      onPick={pick}
      full={selected.length >= limits.gameLink}
      onClose={onEnd}
    />
  )
}
