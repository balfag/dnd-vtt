import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Combatant } from '../../shared/protocol'
import { CombatMenu, type CombatMenuItem } from './CombatMenu'
import { parseInitiative } from './initiative'

interface CombatRowProps {
  combatant: Combatant
  isTurn: boolean
  /** color is the owner's colour, readable on the panel; null: no owner. */
  color: string | null
  isGM: boolean
  /** canSetInitiative: the viewer may type or roll the initiative of this combatant. */
  canSetInitiative: boolean
  /** onSetInitiative resolves true when the server took the value. */
  onSetInitiative: (value: number | null) => Promise<boolean>
  onRoll: () => void
  /** onLook selects the token on the canvas and brings it to the middle. */
  onLook: () => void
  onHide: (hidden: boolean) => void
  onRemove: () => void
}

function DiceIcon() {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      <path d="M12 3l8 4.5v9L12 21l-8-4.5v-9z" />
      <path d="M4 7.5l8 4.5 8-4.5M12 12v9" />
    </svg>
  )
}

/** CombatRow is one combatant: the initiative, the owner's dot, the name, and what the viewer may do. */
export function CombatRow({
  combatant,
  isTurn,
  color,
  isGM,
  canSetInitiative,
  onSetInitiative,
  onRoll,
  onLook,
  onHide,
  onRemove,
}: CombatRowProps) {
  const { t } = useTranslation()
  const name = combatant.name === '' ? t('combat.unnamed') : combatant.name
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const [invalid, setInvalid] = useState(false)
  const [busy, setBusy] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const value = useRef<HTMLButtonElement>(null)
  // The focus goes back to the number when the edit ends by the keyboard.
  const restoreFocus = useRef(false)
  const nameId = useId()
  const rollId = useId()

  useEffect(() => {
    if (editing) {
      input.current?.focus()
      input.current?.select()
    } else if (restoreFocus.current) {
      restoreFocus.current = false
      value.current?.focus()
    }
  }, [editing])

  function startEdit() {
    setDraft(combatant.initiative === null ? '' : String(combatant.initiative))
    setInvalid(false)
    setEditing(true)
  }

  function endEdit(restore: boolean) {
    restoreFocus.current = restore
    setEditing(false)
    setInvalid(false)
  }

  async function submit() {
    const parsed = parseInitiative(draft)
    if (parsed === undefined) {
      setInvalid(true)
      return
    }
    if (parsed === combatant.initiative) {
      endEdit(true)
      return
    }
    setBusy(true)
    try {
      if (await onSetInitiative(parsed)) endEdit(true)
    } finally {
      setBusy(false)
    }
  }

  const shown = combatant.initiative === null ? '—' : String(combatant.initiative)
  const items: CombatMenuItem[] = [
    { id: 'roll', label: t('combat.rollOne'), onSelect: onRoll },
    {
      id: 'hidden',
      label: combatant.hidden ? t('combat.show') : t('combat.hide'),
      onSelect: () => {
        onHide(!combatant.hidden)
      },
    },
    { id: 'remove', label: t('combat.remove'), onSelect: onRemove, danger: true },
  ]

  const className = [
    'combat-row',
    isTurn ? 'combat-row--turn' : '',
    combatant.hidden ? 'combat-row--hidden' : '',
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <li className={className} aria-current={isTurn ? 'true' : undefined}>
      {editing ? (
        <input
          ref={input}
          className="field__input combat-row__input"
          inputMode="numeric"
          aria-label={t('combat.initiative', { name })}
          aria-invalid={invalid}
          value={draft}
          disabled={busy}
          onChange={(e) => {
            setDraft(e.target.value)
            setInvalid(false)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              void submit()
            } else if (e.key === 'Escape') {
              e.preventDefault()
              endEdit(true)
            }
          }}
          onBlur={() => {
            if (!busy) endEdit(false)
          }}
        />
      ) : canSetInitiative ? (
        <button
          ref={value}
          type="button"
          className="combat-row__init combat-row__init--edit"
          aria-label={t('combat.initiative', { name })}
          onClick={startEdit}
        >
          {shown}
        </button>
      ) : (
        <span className="combat-row__init">{shown}</span>
      )}
      <span
        className={combatant.hidden ? 'combat-row__dot combat-row__dot--hidden' : 'combat-row__dot'}
        style={color === null || combatant.hidden ? undefined : { background: color }}
        aria-hidden="true"
      />
      <button type="button" className="combat-row__main" onClick={onLook}>
        <span id={nameId} className="combat-row__name">
          {name}
        </span>
        {isTurn && <span className="combat-row__turn">{t('combat.turn')}</span>}
        {combatant.hidden && <span className="combat-row__hidden">{t('combat.hidden')}</span>}
      </button>
      {canSetInitiative && !editing && (
        <button
          id={rollId}
          type="button"
          className="combat-row__roll"
          aria-label={t('combat.roll')}
          aria-labelledby={`${rollId} ${nameId}`}
          title={t('combat.roll')}
          onClick={onRoll}
        >
          <DiceIcon />
        </button>
      )}
      {isGM && <CombatMenu label={t('combat.menu', { name })} items={items} />}
    </li>
  )
}
