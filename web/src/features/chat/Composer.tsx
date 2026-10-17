import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { parseInput, type ParsedInput } from './parseInput'
import { sendInput, type Command, type Problem, type SendState } from './sendInput'

interface ComposerProps {
  command: Command
  players: { id: string; displayName: string }[]
  meId: string
}

const quickDice = ['d4', 'd6', 'd8', 'd10', 'd12', 'd20', 'd100']
/** Composer is the quick roll buttons and the input line of the chat. */
export function Composer({ command, players, meId }: ComposerProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const [problem, setProblem] = useState<Problem | null>(null)
  const state = useRef<SendState>({ inFlight: null, lastSent: '' })
  const hooks = { setText, setProblem }

  function submit() {
    const parsed = parseInput(text, players, meId)
    sendInput(command, state.current, hooks, text, parsed, true)
  }

  function quick(formula: string) {
    const parsed: ParsedInput = { kind: 'roll', formula, label: '', visibility: 'public' }
    sendInput(command, state.current, hooks, `/r ${formula}`, parsed, false)
  }
  const hintId = 'chat-input-hint'
  return (
    <div className="composer">
      <div className="composer__quick" role="group" aria-label={t('chat.quick.label')}>
        {quickDice.map((die) => (
          <button
            key={die}
            type="button"
            className="composer__die"
            onClick={() => {
              quick(`1${die}`)
            }}
          >
            {die}
          </button>
        ))}
        <button
          type="button"
          className="composer__die composer__die--wide"
          onClick={() => {
            quick('2d20kh1')
          }}
        >
          {t('chat.quick.adv')}
        </button>
        <button
          type="button"
          className="composer__die composer__die--wide"
          onClick={() => {
            quick('2d20kl1')
          }}
        >
          {t('chat.quick.dis')}
        </button>
      </div>
      <form
        className="composer__form"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <input
          className="field__input composer__input"
          type="text"
          autoComplete="off"
          aria-label={t('chat.input.label')}
          aria-describedby={hintId}
          aria-invalid={problem !== null}
          placeholder={t('chat.input.placeholder')}
          value={text}
          onChange={(e) => {
            setText(e.target.value)
            setProblem(null)
          }}
          onKeyDown={(e) => {
            if (e.key === 'ArrowUp' && text === '' && state.current.lastSent !== '') {
              e.preventDefault()
              setText(state.current.lastSent)
            }
          }}
        />
      </form>
      {problem === null ? (
        <p id={hintId} className="composer__note">
          {t('chat.input.hint')}
        </p>
      ) : (
        <p id={hintId} className="composer__note composer__note--error" role="alert">
          {t(problem.key, problem.params)}
        </p>
      )}
    </div>
  )
}
