import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { errorCodes } from '../../shared/api/errors'
import { MessageType } from '../../shared/protocol'
import { CommandError } from '../../shared/ws/errors'
import { parseInput, type ParsedInput } from './parseInput'

type Command = (type: string, data?: unknown) => Promise<unknown>

interface ComposerProps {
  command: Command
  players: { id: string; displayName: string }[]
  meId: string
}

interface Problem {
  key: string
  params?: Record<string, string>
}

const quickDice = ['d4', 'd6', 'd8', 'd10', 'd12', 'd20', 'd100']

function commandProblem(err: unknown, formula: string | null): Problem {
  if (err instanceof CommandError) {
    if (err.code === 'rate_limited') return { key: 'chat.error.tooFast' }
    if (err.code === 'bad_request' && formula !== null) {
      return { key: 'chat.error.formula', params: { formula } }
    }
    const known = errorCodes.find((code) => code === err.code)
    if (known !== undefined) return { key: `errors.${known}` }
  }
  return { key: 'errors.unknown' }
}

function send(command: Command, parsed: ParsedInput): Promise<unknown> {
  switch (parsed.kind) {
    case 'text':
      return command(MessageType.ChatSend, { text: parsed.text })
    case 'private':
      return command(MessageType.ChatSend, { text: parsed.text, to: parsed.to })
    case 'roll':
      return command(MessageType.RollRequest, {
        formula: parsed.formula,
        visibility: parsed.visibility,
        ...(parsed.label === '' ? {} : { label: parsed.label }),
      })
    case 'error':
      return Promise.reject(new Error(parsed.key))
  }
}

/** Composer is the quick roll buttons and the input line of the chat. */
export function Composer({ command, players, meId }: ComposerProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const [problem, setProblem] = useState<Problem | null>(null)
  const inFlight = useRef<string | null>(null)
  const lastSent = useRef('')

  function run(input: string, parsed: ParsedInput, onDone?: () => void) {
    if (parsed.kind === 'error') {
      setProblem({
        key: parsed.key,
        ...(parsed.params === undefined ? {} : { params: parsed.params }),
      })
      return
    }
    // The same text is not sent again until the server has answered.
    if (inFlight.current === input) return
    inFlight.current = input
    setProblem(null)
    const formula = parsed.kind === 'roll' ? parsed.formula : null
    send(command, parsed)
      .then(() => {
        lastSent.current = input
        onDone?.()
      })
      .catch((err: unknown) => {
        setProblem(commandProblem(err, formula))
      })
      .finally(() => {
        if (inFlight.current === input) inFlight.current = null
      })
  }

  function submit() {
    const input = text
    run(input, parseInput(input, players, meId), () => {
      // A text edited while the request was on its way stays.
      setText((current) => (current === input ? '' : current))
    })
  }

  function quick(formula: string) {
    run(`/r ${formula}`, { kind: 'roll', formula, label: '', visibility: 'public' })
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
            if (e.key === 'ArrowUp' && text === '' && lastSent.current !== '') {
              e.preventDefault()
              setText(lastSent.current)
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
